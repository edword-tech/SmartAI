package probe

import (
	"context"
	"testing"

	"flexai/unified-resource-manager/core/pool"
)

func testPool(t *testing.T) *pool.UnifiedResourcePool {
	t.Helper()
	gpu := pool.NewLedger(pool.ResourceTypeGPU)
	npu := pool.NewLedger(pool.ResourceTypeNPU)
	gpu.SyncNode("node-gpu-1", []pool.DeviceSpec{
		{DeviceID: "0", Model: "A100", TotalMemory: 40, TotalCores: 100},
		{DeviceID: "7", Model: "A100", TotalMemory: 40, TotalCores: 100},
	})
	npu.SyncNode("node-npu-1", []pool.DeviceSpec{
		{DeviceID: "0", Model: "910B3", TotalMemory: 32, TotalCores: 10},
		{DeviceID: "7", Model: "910B3", TotalMemory: 32, TotalCores: 10},
	})
	u := pool.NewUnifiedResourcePool()
	u.RegisterPool("gpu", pool.NewLedgerPool(gpu, "gpu", pool.ResourceTypeGPU, nil, nil))
	u.RegisterPool("npu", pool.NewLedgerPool(npu, "npu", pool.ResourceTypeNPU, nil, nil))
	return u
}

func TestReserveAndProbeGPUAndNPU(t *testing.T) {
	ctx := context.Background()
	u := testPool(t)
	ids, err := ReserveCapacity(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 reserved, got %v", ids)
	}

	// 业务任务不能吃掉调测卡：两张 GPU 里一张预留，业务最多分到 1 张
	if _, err := u.AllocateResources(ctx, &pool.AllocationRequest{
		TaskID: "biz", ResourceType: pool.ResourceTypeGPU, Count: 2, Cores: 100,
	}); err == nil {
		t.Fatal("business alloc should not consume debug-reserved GPU")
	}
	biz, err := u.AllocateResources(ctx, &pool.AllocationRequest{
		TaskID: "biz", ResourceType: pool.ResourceTypeGPU, Count: 1, Cores: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if biz.Slices[0].ResourceID == "" {
		t.Fatal("empty business allocation")
	}

	m := New(u)
	rep := m.Probe(ctx, nil)
	if rep.Status != StatusPass {
		t.Fatalf("probe failed: %+v", rep)
	}
	if len(rep.Results) != 2 {
		t.Fatalf("want gpu+npu, got %d", len(rep.Results))
	}
	for _, r := range rep.Results {
		if !r.Ready || r.AllocatedID == "" {
			t.Fatalf("incomplete result: %+v", r)
		}
		if r.ReservedID != "" && r.AllocatedID != r.ReservedID {
			t.Fatalf("%s probe used %s, not reserved %s", r.ResourceType, r.AllocatedID, r.ReservedID)
		}
	}
	// 释放后调测卡仍在，且按类型计数不丢卡
	ready := m.Readiness(ctx)
	if len(ready) != 2 {
		t.Fatalf("want gpu+npu readiness, got %d", len(ready))
	}
	for _, r := range ready {
		if !r.Ready || r.ReservedReady < 1 || r.Total != 2 {
			t.Fatalf("readiness %+v", r)
		}
	}
	_ = u.ReleaseResources(ctx, biz.AllocationID)
}
