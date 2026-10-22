package pool

import (
	"context"
	"testing"
)

func newTestLedger() *Ledger {
	l := NewLedger(ResourceTypeGPU)
	l.SyncNode("n1", []DeviceSpec{
		{DeviceID: "0", Model: "A100", TotalMemory: 40, TotalCores: 100},
		{DeviceID: "1", Model: "RTX 4090", TotalMemory: 24, TotalCores: 100},
	})
	return l
}

func TestLedgerSharedAllocationPacksAndReleases(t *testing.T) {
	l := newTestLedger()
	req := &AllocationRequest{Count: 1, MemoryGB: 10, Cores: 30, ResourceIDs: []string{"n1-0"}}
	a1, _, err := l.Allocate(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	a2, _, err := l.Allocate(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := l.Get("n1-0")
	if r.UsedMemory != 20 || r.UsedCores != 60 || r.ShareMode != ShareModeShared || r.Holders != 2 {
		t.Fatalf("unexpected usage %+v", r)
	}
	if _, _, err := l.Allocate(&AllocationRequest{Count: 1, ShareMode: ShareModeExclusive, ResourceIDs: []string{"n1-0"}}, nil); err == nil {
		t.Fatal("exclusive allocation on shared device should fail")
	}
	if _, err := l.Release(a1.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Release(a2.ID); err != nil {
		t.Fatal(err)
	}
	r, _ = l.Get("n1-0")
	if r.UsedMemory != 0 || r.ShareMode != "" || r.State != StateAvailable {
		t.Fatalf("device not reset after release: %+v", r)
	}
	if _, err := l.Release(a1.ID); err != ErrAllocationNotFound {
		t.Fatalf("double release should return ErrAllocationNotFound, got %v", err)
	}
}

func TestLedgerMIGProfiles(t *testing.T) {
	l := newTestLedger()
	req := &AllocationRequest{Count: 1, MIGProfile: "3g.20gb", ResourceIDs: []string{"n1-0"}}
	for i := 0; i < 2; i++ {
		if _, _, err := l.Allocate(req, nil); err != nil {
			t.Fatalf("mig alloc %d: %v", i, err)
		}
	}
	// 6/7 compute slices, 40/40 GB used
	if _, _, err := l.Allocate(&AllocationRequest{Count: 1, MIGProfile: "1g.5gb", ResourceIDs: []string{"n1-0"}}, nil); err == nil {
		t.Fatal("expected no memory left for 1g.5gb")
	}
	r, _ := l.Get("n1-0")
	if r.MIGSlices != 6 || r.State != StateAvailable {
		t.Fatalf("unexpected MIG state %+v", r)
	}
	if _, _, err := l.Allocate(&AllocationRequest{Count: 1, MIGProfile: "1g.5gb", ResourceIDs: []string{"n1-1"}}, nil); err == nil {
		t.Fatal("RTX 4090 must not support MIG")
	}
}

func TestLedgerTimeSliceOvercommit(t *testing.T) {
	l := newTestLedger()
	l.SetCoreOvercommit(2)
	req := &AllocationRequest{Count: 1, ShareMode: ShareModeTimeSlice, MemoryGB: 4, Cores: 100, ResourceIDs: []string{"n1-1"}}
	for i := 0; i < 2; i++ {
		if _, _, err := l.Allocate(req, nil); err != nil {
			t.Fatalf("timeslice alloc %d: %v", i, err)
		}
	}
	if _, _, err := l.Allocate(req, nil); err == nil {
		t.Fatal("third 100%% timeslice allocation should exceed 2x overcommit")
	}
}

func TestLedgerCordonAndBinpack(t *testing.T) {
	l := newTestLedger()
	if err := l.SetCordon("n1-0", true); err != nil {
		t.Fatal(err)
	}
	_, res, err := l.Allocate(&AllocationRequest{Count: 1, MemoryGB: 8, Cores: 20}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res[0].ID != "n1-1" {
		t.Fatalf("cordoned device must be skipped, got %s", res[0].ID)
	}
	// 第二个切分请求应装箱到已占用的 n1-1
	_ = l.SetCordon("n1-0", false)
	_, res, err = l.Allocate(&AllocationRequest{Count: 1, MemoryGB: 8, Cores: 20}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res[0].ID != "n1-1" {
		t.Fatalf("binpack should prefer partially used device, got %s", res[0].ID)
	}
}

func TestDeviceFitSkipsDebugReserved(t *testing.T) {
	l := newTestLedger()
	if err := l.SetDeviceLabel("n1-0", LabelPurpose, PurposeDebug); err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.Allocate(&AllocationRequest{Count: 1, ShareMode: ShareModeExclusive, ResourceIDs: []string{"n1-0"}}, nil); err == nil {
		t.Fatal("business exclusive alloc must not take debug-reserved device")
	}
	if _, res, err := l.Allocate(&AllocationRequest{Count: 1, ShareMode: ShareModeExclusive}, nil); err != nil {
		t.Fatal(err)
	} else if res[0].ID != "n1-1" {
		t.Fatalf("business should land on free card, got %s", res[0].ID)
	}
	dbg, _, err := l.Allocate(&AllocationRequest{
		Count: 1, ShareMode: ShareModeExclusive,
		Labels: map[string]string{LabelPurpose: PurposeDebug},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if dbg.Slices[0].ResourceID != "n1-0" {
		t.Fatalf("debug alloc should prefer reserved n1-0, got %s", dbg.Slices[0].ResourceID)
	}
}

func TestUnifiedPoolAllocateReleaseNoDeadlock(t *testing.T) {
	l := newTestLedger()
	u := NewUnifiedResourcePool()
	u.RegisterPool("gpu", NewLedgerPool(l, "gpu", ResourceTypeGPU, nil, nil))
	ctx := context.Background()
	res, err := u.AllocateAny(ctx, &AllocationRequest{ResourceType: ResourceTypeGPU, Count: 2, ShareMode: ShareModeExclusive})
	if err != nil {
		t.Fatal(err)
	}
	if res.PoolID != "gpu" || len(u.ListAllocations("gpu")) != 1 {
		t.Fatalf("allocation not recorded: %+v", res)
	}
	if err := u.ReleaseResources(ctx, res.AllocationID); err != nil {
		t.Fatal(err)
	}
	if len(u.ListAllocations("gpu")) != 0 {
		t.Fatal("allocation record not removed")
	}
}
