package remediation

import (
	"testing"
	"time"

	"flexai/unified-resource-manager/core/pool"
	"flexai/unified-resource-manager/ops/telemetry"
)

type fakeActuator struct {
	cordoned    map[string]bool
	health      map[string]string
	rescheduled []string
	nodes       []string
}

func newFake() *fakeActuator {
	return &fakeActuator{cordoned: map[string]bool{}, health: map[string]string{}}
}

func (f *fakeActuator) CordonDevice(id string, c bool) error { f.cordoned[id] = c; return nil }
func (f *fakeActuator) SetDeviceHealth(id, h string) error   { f.health[id] = h; return nil }
func (f *fakeActuator) RescheduleDevice(id, _ string) []string {
	f.rescheduled = append(f.rescheduled, id)
	return []string{"task-" + id}
}
func (f *fakeActuator) RescheduleNode(n, _ string) []string {
	f.nodes = append(f.nodes, n)
	return []string{"t1", "t2"}
}

func sample(id string, mut func(*telemetry.DeviceTelemetry)) map[string]*telemetry.DeviceTelemetry {
	s := &telemetry.DeviceTelemetry{ResourceID: id, NodeID: "n1", TemperatureC: 50, Timestamp: time.Now()}
	if mut != nil {
		mut(s)
	}
	return map[string]*telemetry.DeviceTelemetry{id: s}
}

func TestFatalXIDIsolatesAndNeedsReset(t *testing.T) {
	act := newFake()
	e := NewEngine(act)
	e.Evaluate(sample("n1-0", nil))
	e.Evaluate(sample("n1-0", func(s *telemetry.DeviceTelemetry) { s.XID = 79 }))

	if !act.cordoned["n1-0"] || act.health["n1-0"] != pool.DeviceHealthFailed {
		t.Fatalf("expected cordon+failed, got %+v %+v", act.cordoned, act.health)
	}
	if len(act.rescheduled) != 1 {
		t.Fatalf("expected one reschedule, got %v", act.rescheduled)
	}
	// 重复样本不重复处置
	e.Evaluate(sample("n1-0", func(s *telemetry.DeviceTelemetry) { s.XID = 79 }))
	if len(act.rescheduled) != 1 {
		t.Fatalf("duplicate remediation: %v", act.rescheduled)
	}
	// 致命故障不会自动恢复
	for i := 0; i < 5; i++ {
		e.Evaluate(sample("n1-0", nil))
	}
	if !act.cordoned["n1-0"] {
		t.Fatal("fatal fault must not auto-recover")
	}
	st := e.Stats()
	if st.PendingManual != 1 || st.FaultsDetected != 1 {
		t.Fatalf("unexpected stats %+v", st)
	}
	if err := e.ResetDevice("n1-0", "ops"); err != nil {
		t.Fatal(err)
	}
	if act.cordoned["n1-0"] || act.health["n1-0"] != pool.DeviceHealthHealthy {
		t.Fatal("reset should restore device")
	}
	if st := e.Stats(); st.ManualResets != 1 || st.PendingManual != 0 {
		t.Fatalf("unexpected stats after reset %+v", st)
	}
}

func TestThermalDegradeAndAutoRecover(t *testing.T) {
	act := newFake()
	e := NewEngine(act)
	e.Evaluate(sample("n1-1", func(s *telemetry.DeviceTelemetry) { s.TemperatureC = 88 }))
	if act.health["n1-1"] != pool.DeviceHealthDegraded || act.cordoned["n1-1"] {
		t.Fatalf("expected degraded without cordon, got %+v %+v", act.health, act.cordoned)
	}
	// 升级为严重：隔离 + 迁移
	e.Evaluate(sample("n1-1", func(s *telemetry.DeviceTelemetry) { s.TemperatureC = 97 }))
	if !act.cordoned["n1-1"] || len(act.rescheduled) != 1 {
		t.Fatalf("expected escalation to isolate, got %+v %v", act.cordoned, act.rescheduled)
	}
	for i := 0; i < 3; i++ {
		e.Evaluate(sample("n1-1", nil))
	}
	if act.cordoned["n1-1"] || act.health["n1-1"] != pool.DeviceHealthHealthy {
		t.Fatalf("expected auto recovery, got %+v %+v", act.cordoned, act.health)
	}
	st := e.Stats()
	if st.AutoRecovered != 1 || st.SelfHealRate != 1 {
		t.Fatalf("unexpected stats %+v", st)
	}
}

func TestAlertOnlyAndCounterDeltas(t *testing.T) {
	act := newFake()
	e := NewEngine(act)
	off := false
	if _, err := e.UpdateRule(RuleNVLinkCRC, RuleUpdate{AutoRemediate: &off}); err != nil {
		t.Fatal(err)
	}
	// 首个样本只建立基线，存量计数不触发
	e.Evaluate(sample("n1-2", func(s *telemetry.DeviceTelemetry) { s.NVLinkCRCErrors = 10000 }))
	if len(e.Events(EventFilter{})) != 0 {
		t.Fatal("baseline sample should not fire")
	}
	e.Evaluate(sample("n1-2", func(s *telemetry.DeviceTelemetry) { s.NVLinkCRCErrors = 10100 }))
	evs := e.Events(EventFilter{ResourceID: "n1-2"})
	if len(evs) != 1 || evs[0].RuleID != RuleNVLinkCRC {
		t.Fatalf("expected nvlink alert, got %+v", evs)
	}
	if _, ok := act.health["n1-2"]; ok {
		t.Fatal("alert-only rule must not act")
	}
}

func TestNodeFailure(t *testing.T) {
	act := newFake()
	e := NewEngine(act)
	e.HandleNodeFailure("n9", "heartbeat lost")
	if len(act.nodes) != 1 {
		t.Fatal("expected node reschedule")
	}
	if st := e.Stats(); st.TasksRescheduled != 2 {
		t.Fatalf("unexpected stats %+v", st)
	}
}
