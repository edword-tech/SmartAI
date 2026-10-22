package scheduler

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"flexai/unified-resource-manager/core/pool"
)

type fakeInspector struct {
	cordoned map[string]bool
	labels   map[string]map[string]string
}

func (f *fakeInspector) IsSchedulable(n string) bool           { return !f.cordoned[n] }
func (f *fakeInspector) NodeLabels(n string) map[string]string { return f.labels[n] }

// eightGPUNode 8 卡节点：0-3 与 4-7 各为一个 NVLink 组，分别位于 NUMA 0/1
func eightGPUNode(model string, mem int64) []pool.DeviceSpec {
	var specs []pool.DeviceSpec
	for i := 0; i < 8; i++ {
		specs = append(specs, pool.DeviceSpec{
			DeviceID: fmt.Sprint(i), Model: model, TotalMemory: mem, TotalCores: 100,
			Topology: &pool.DeviceTopology{NUMANode: i / 4, NVLinkGroup: fmt.Sprintf("nvl%d", i/4)},
		})
	}
	return specs
}

func fourGPUNode(model string, mem int64) []pool.DeviceSpec {
	return eightGPUNode(model, mem)[:4]
}

type env struct {
	ledger *pool.Ledger
	sched  *IntelligentScheduler
	insp   *fakeInspector
	now    time.Time
}

func newEnv(t *testing.T, nodes map[string][]pool.DeviceSpec) *env {
	t.Helper()
	l := pool.NewLedger(pool.ResourceTypeGPU)
	for n, specs := range nodes {
		l.SyncNode(n, specs)
	}
	u := pool.NewUnifiedResourcePool()
	u.RegisterPool("gpu", pool.NewLedgerPool(l, "gpu", pool.ResourceTypeGPU, nil, nil))
	s := NewIntelligentScheduler(u, &BinpackStrategy{})
	e := &env{ledger: l, sched: s, insp: &fakeInspector{cordoned: map[string]bool{}, labels: map[string]map[string]string{}}, now: time.Unix(1_700_000_000, 0)}
	s.SetNodeInspector(e.insp)
	s.now = func() time.Time { return e.now }
	return e
}

func (e *env) submit(t *testing.T, task *Task) *Task {
	t.Helper()
	if err := e.sched.SubmitTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.Add(time.Millisecond)
	return task
}

func gpuReq(count int, mem int64, cores int) *pool.AllocationRequest {
	return &pool.AllocationRequest{ResourceType: pool.ResourceTypeGPU, Count: count, MemoryGB: mem, Cores: cores}
}

func whole(count int) *pool.AllocationRequest { return gpuReq(count, 0, 100) }

func TestScheduleAndCancelReleasesDevices(t *testing.T) {
	e := newEnv(t, map[string][]pool.DeviceSpec{"n1": fourGPUNode("A100", 40)})
	task := e.submit(t, &Task{ID: "t1", Type: TaskTypeTraining, ResourceReq: whole(2)})
	e.sched.RunCycle()
	if task.Status != TaskStatusRunning || len(task.Slices) != 2 || task.PoolID != "gpu" {
		t.Fatalf("task not running: %+v", task)
	}
	if err := e.sched.CancelTask(context.Background(), "t1"); err != nil {
		t.Fatal(err)
	}
	for _, r := range e.ledger.List(nil) {
		if r.Holders != 0 {
			t.Fatalf("device %s still held after cancel", r.ID)
		}
	}
}

func TestTopologyAwarePlacementPrefersIntactNVLinkGroup(t *testing.T) {
	e := newEnv(t, map[string][]pool.DeviceSpec{"n1": eightGPUNode("A100", 80)})
	small := e.submit(t, &Task{ID: "small", Type: TaskTypeTraining, ResourceReq: whole(1)})
	e.sched.RunCycle()
	big := e.submit(t, &Task{ID: "big", Type: TaskTypeTraining, ResourceReq: whole(4)})
	e.sched.RunCycle()
	if big.Status != TaskStatusRunning {
		t.Fatalf("big not running: %s", big.Message)
	}
	group := func(id string) string { r, _ := e.ledger.Get(id); return r.Topology.NVLinkGroup }
	g := group(big.Slices[0].ResourceID)
	for _, s := range big.Slices {
		if group(s.ResourceID) != g {
			t.Fatalf("4-GPU job spans NVLink groups: %+v", big.Slices)
		}
	}
	if g == group(small.Slices[0].ResourceID) {
		t.Fatalf("4-GPU job should use the intact NVLink group, got %s", g)
	}
}

func TestSharedInferencePacksOntoSameCard(t *testing.T) {
	e := newEnv(t, map[string][]pool.DeviceSpec{"n1": fourGPUNode("A100", 40)})
	a := e.submit(t, &Task{ID: "a", ResourceReq: gpuReq(1, 10, 25)})
	b := e.submit(t, &Task{ID: "b", ResourceReq: gpuReq(1, 10, 25)})
	e.sched.RunCycle()
	if a.Status != TaskStatusRunning || b.Status != TaskStatusRunning {
		t.Fatalf("not running: %s / %s", a.Message, b.Message)
	}
	if a.Slices[0].ResourceID != b.Slices[0].ResourceID {
		t.Fatalf("shared instances should be bin-packed: %s vs %s", a.Slices[0].ResourceID, b.Slices[0].ResourceID)
	}
}

func TestGangAllOrNothing(t *testing.T) {
	e := newEnv(t, map[string][]pool.DeviceSpec{"n1": fourGPUNode("A100", 80), "n2": fourGPUNode("A100", 80)})
	e.insp.labels["n1"] = map[string]string{LabelRDMADomain: "leaf-1"}
	e.insp.labels["n2"] = map[string]string{LabelRDMADomain: "leaf-1"}
	var members []*Task
	for i := 0; i < 3; i++ {
		members = append(members, e.submit(t, &Task{ID: fmt.Sprintf("w%d", i), Type: TaskTypeTraining, GangGroup: "job", MinMember: 3, ResourceReq: whole(4)}))
	}
	e.sched.RunCycle()
	for _, m := range members {
		if m.Status != TaskStatusPending {
			t.Fatalf("gang member %s should wait (only 2 nodes for 3 workers)", m.ID)
		}
	}
	for _, r := range e.ledger.List(nil) {
		if r.Holders != 0 {
			t.Fatal("gang must not partially allocate")
		}
	}
	_ = e.sched.CancelTask(context.Background(), "w2")
	for _, m := range members[:2] {
		m.MinMember = 2
	}
	e.sched.RunCycle()
	if members[0].Status != TaskStatusRunning || members[1].Status != TaskStatusRunning {
		t.Fatalf("gang of 2 should run: %s", members[0].Message)
	}
	if members[0].Nodes[0] == members[1].Nodes[0] {
		t.Fatal("gang workers should occupy different nodes")
	}
}

func TestPreemptionRequeuesVictim(t *testing.T) {
	e := newEnv(t, map[string][]pool.DeviceSpec{"n1": fourGPUNode("A100", 40)})
	low := e.submit(t, &Task{ID: "low", Type: TaskTypeTraining, Priority: PriorityLow, ResourceReq: whole(4)})
	e.sched.RunCycle()
	high := e.submit(t, &Task{ID: "high", Type: TaskTypeInference, Priority: PriorityHigh, ResourceReq: whole(1)})
	e.sched.RunCycle()
	if high.Status != TaskStatusRunning {
		t.Fatalf("high priority task should preempt: %s", high.Message)
	}
	if low.Status != TaskStatusPending || low.PreemptionCount != 1 {
		t.Fatalf("victim should be requeued, got %s (%d)", low.Status, low.PreemptionCount)
	}
}

func TestQueueCapabilityAndFairShare(t *testing.T) {
	e := newEnv(t, map[string][]pool.DeviceSpec{"n1": fourGPUNode("A100", 40)})
	q := e.sched.Queues()
	_ = q.Upsert(&Queue{Name: "qa", Weight: 1})
	_ = q.Upsert(&Queue{Name: "qb", Weight: 1})
	_ = q.Upsert(&Queue{Name: "capped", Weight: 1, Capability: ResourceList{GPU: 1}})

	e.submit(t, &Task{ID: "a1", Queue: "qa", ResourceReq: whole(1)})
	e.submit(t, &Task{ID: "a2", Queue: "qa", ResourceReq: whole(1)})
	e.sched.RunCycle()

	c1 := e.submit(t, &Task{ID: "c1", Queue: "capped", ResourceReq: whole(1)})
	c2 := e.submit(t, &Task{ID: "c2", Queue: "capped", ResourceReq: whole(1)})
	e.sched.RunCycle()
	if c1.Status != TaskStatusRunning || c2.Status != TaskStatusPending || !strings.Contains(c2.Message, "capability") {
		t.Fatalf("capability not enforced: c1=%s c2=%s (%s)", c1.Status, c2.Status, c2.Message)
	}
	_ = e.sched.CancelTask(context.Background(), "c2")

	// 剩 1 张卡：qa 已占 2 张，qb 为 0，同优先级下 qb 先调度
	a3 := e.submit(t, &Task{ID: "a3", Queue: "qa", ResourceReq: whole(1)})
	b1 := e.submit(t, &Task{ID: "b1", Queue: "qb", ResourceReq: whole(1)})
	e.sched.RunCycle()
	if b1.Status != TaskStatusRunning || a3.Status != TaskStatusPending {
		t.Fatalf("fair share violated: a3=%s b1=%s", a3.Status, b1.Status)
	}
}

func TestFaultRescheduleMovesTaskOffFailedDevice(t *testing.T) {
	e := newEnv(t, map[string][]pool.DeviceSpec{"n1": fourGPUNode("A100", 40)})
	task := e.submit(t, &Task{ID: "t", Type: TaskTypeTraining, ResourceReq: whole(1), Labels: map[string]string{"checkpoint.auto_resume": "true"}})
	e.sched.RunCycle()
	failed := task.Slices[0].ResourceID
	_ = e.ledger.SetHealth(failed, pool.DeviceHealthFailed)
	affected := e.sched.HandleResourceFailure([]string{failed}, "XID 79")
	if len(affected) != 1 || task.RetryCount != 1 || !strings.Contains(task.Message, "checkpoint") {
		t.Fatalf("unexpected reschedule result %v %+v", affected, task)
	}
	e.sched.RunCycle()
	if task.Status != TaskStatusRunning || task.Slices[0].ResourceID == failed {
		t.Fatalf("task should run on a healthy device, got %s on %v", task.Status, task.Slices)
	}
}

func TestElasticDegradeAndScaleUp(t *testing.T) {
	e := newEnv(t, map[string][]pool.DeviceSpec{"n1": fourGPUNode("A100", 40)})
	e.submit(t, &Task{ID: "other", Type: TaskTypeTraining, ResourceReq: whole(2)})
	e.sched.RunCycle()
	el := e.submit(t, &Task{ID: "el", Type: TaskTypeTraining, MinCount: 2, ResourceReq: whole(4)})
	e.sched.RunCycle()
	if el.Status != TaskStatusRunning || len(el.Slices) != 2 {
		t.Fatalf("elastic task should start degraded with 2 GPUs, got %s %d (%s)", el.Status, len(el.Slices), el.Message)
	}
	_ = e.sched.CancelTask(context.Background(), "other")
	e.sched.RunCycle()
	if len(el.Slices) != 4 {
		t.Fatalf("elastic task should scale up to 4, got %d", len(el.Slices))
	}
}

func TestDefragmentationFreesWholeCard(t *testing.T) {
	e := newEnv(t, map[string][]pool.DeviceSpec{"n1": fourGPUNode("A100", 40)[:2]})
	a := e.submit(t, &Task{ID: "a", ResourceReq: gpuReq(1, 10, 25)})
	e.sched.RunCycle()
	// 手工把第二个实例放到另一张卡上制造碎片
	b := e.submit(t, &Task{ID: "b", ResourceReq: gpuReq(1, 10, 25)})
	other := "n1-0"
	if a.Slices[0].ResourceID == other {
		other = "n1-1"
	}
	b.ResourceReq.ConstrainNodeIDs = nil
	e.ledger.SetCordon(a.Slices[0].ResourceID, true)
	e.sched.RunCycle()
	e.ledger.SetCordon(a.Slices[0].ResourceID, false)
	if b.Slices[0].ResourceID != other {
		t.Fatalf("setup failed: %v", b.Slices)
	}

	plan := e.sched.PlanDefragmentation()
	if len(plan.Moves) != 1 || len(plan.FreedDevices) != 1 {
		t.Fatalf("expected 1 move freeing 1 card, got %+v", plan)
	}
	if _, err := e.sched.ExecuteDefragmentation(); err != nil {
		t.Fatal(err)
	}
	if a.Slices[0].ResourceID != b.Slices[0].ResourceID {
		t.Fatal("instances should be consolidated onto one card")
	}
	rep := e.sched.AnalyzeFragmentation()
	if rep.Stats[0].IdleDevices != 1 {
		t.Fatalf("expected one idle card after defrag, got %+v", rep.Stats[0])
	}
}

func TestTenantIsolationAndCordon(t *testing.T) {
	e := newEnv(t, map[string][]pool.DeviceSpec{"n1": fourGPUNode("A100", 40), "n2": fourGPUNode("A100", 40)})
	e.insp.labels["n1"] = map[string]string{LabelTenantDedicated: "tenant-vip"}
	e.insp.cordoned["n2"] = true
	task := e.submit(t, &Task{ID: "t", TenantID: "tenant-other", ResourceReq: whole(1)})
	e.sched.RunCycle()
	if task.Status != TaskStatusPending || !strings.Contains(task.Message, "TenantIsolation") || !strings.Contains(task.Message, "NodeSchedulable") {
		t.Fatalf("expected isolation + cordon to block scheduling, got %s: %s", task.Status, task.Message)
	}
	vip := e.submit(t, &Task{ID: "vip", TenantID: "tenant-vip", ResourceReq: whole(1)})
	e.sched.RunCycle()
	if vip.Status != TaskStatusRunning || vip.Nodes[0] != "n1" {
		t.Fatalf("dedicated tenant should land on n1: %s", vip.Message)
	}
}

func TestExpectedDurationCompletesAndFiresHooks(t *testing.T) {
	e := newEnv(t, map[string][]pool.DeviceSpec{"n1": fourGPUNode("A100", 40)})
	var events []LifecycleEventType
	e.sched.AddLifecycleHook(func(ev LifecycleEvent) { events = append(events, ev.Type) })
	task := e.submit(t, &Task{ID: "t", ExpectedDurationSeconds: 10, ResourceReq: whole(1)})
	e.sched.RunCycle()
	e.now = e.now.Add(11 * time.Second)
	e.sched.RunCycle()
	if task.Status != TaskStatusCompleted {
		t.Fatalf("task should complete, got %s", task.Status)
	}
	if len(events) != 2 || events[0] != LifecycleStarted || events[1] != LifecycleFinished {
		t.Fatalf("unexpected lifecycle events %v", events)
	}
}
