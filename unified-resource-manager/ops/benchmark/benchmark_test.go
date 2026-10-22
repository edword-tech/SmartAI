package benchmark

import (
	"strings"
	"testing"

	"flexai/unified-resource-manager/core/pool"
	"flexai/unified-resource-manager/ops/telemetry"
)

type inv struct {
	devices []*pool.Resource
	nics    []NIC
}

func (i inv) NodeDevices(string) []*pool.Resource { return i.devices }
func (i inv) NodeNICs(string) []NIC               { return i.nics }

type teleMap map[string]*telemetry.DeviceTelemetry

func (t teleMap) Get(id string) (*telemetry.DeviceTelemetry, bool) { v, ok := t[id]; return v, ok }

func fixture() inv {
	topo := &pool.DeviceTopology{NVLinkGroup: "nvl-0"}
	return inv{
		devices: []*pool.Resource{
			{ID: "n1-0", NodeID: "n1", DeviceID: "0", Model: "NVIDIA A100 40GB", Topology: topo},
			{ID: "n1-1", NodeID: "n1", DeviceID: "1", Model: "NVIDIA A100 40GB", Topology: topo},
		},
		nics: []NIC{{Name: "mlx5_0", RateGbps: 200, State: "up"}, {Name: "mlx5_1", RateGbps: 200, State: "down"}},
	}
}

func TestSimulatedAcceptanceAttribution(t *testing.T) {
	tele := teleMap{
		"n1-0": {ResourceID: "n1-0", TemperatureC: 50},
		"n1-1": {ResourceID: "n1-1", TemperatureC: 92, Throttle: []string{"hw_thermal_slowdown"}},
	}
	m := NewManager(fixture(), tele, nil)
	var gotPassed *bool
	m.OnAcceptance(func(_ string, passed bool, _ *Run) { gotPassed = &passed })

	run, err := m.RunSimulated("n1", "acceptance", nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.Verdict != StatusFail || gotPassed == nil || *gotPassed {
		t.Fatalf("expected failed acceptance, got %s", run.Verdict)
	}
	byKey := map[string]Result{}
	for _, r := range run.Results {
		byKey[string(r.Test)+"/"+r.Target] = r
	}
	if r := byKey["gpu_burn/0"]; r.Status != StatusPass {
		t.Fatalf("healthy GPU should pass: %+v", r)
	}
	if r := byKey["gpu_burn/1"]; r.Status != StatusFail || !strings.Contains(r.Attribution, "温度墙") {
		t.Fatalf("hot GPU should fail with thermal attribution: %+v", r)
	}
	if r := byKey["rdma_bw/mlx5_1"]; r.Status != StatusFail || !strings.Contains(r.Attribution, "down") {
		t.Fatalf("down NIC should fail: %+v", r)
	}
	if r := byKey["nccl_allreduce/node"]; r.Status != StatusPass {
		t.Fatalf("nccl should pass on healthy NVLink: %+v", r)
	}
}

func TestIngestAndCalibrate(t *testing.T) {
	m := NewManager(fixture(), teleMap{}, nil)
	run, err := m.Ingest("n1", "benchmark", []IngestInput{
		{Test: TestGPUBurn, Target: "0", Value: 250},
		{Test: TestGPUBurn, Target: "1", Value: 246},
		{Test: TestNCCLAllReduce, Target: "node", Value: 120},
	})
	if err != nil {
		t.Fatal(err)
	}
	var nccl Result
	for _, r := range run.Results {
		if r.Test == TestNCCLAllReduce {
			nccl = r
		}
	}
	if nccl.Status != StatusFail || !strings.Contains(nccl.Attribution, "NCCL") {
		t.Fatalf("low busbw should fail with config attribution: %+v", nccl)
	}
	updated, err := m.Calibrate(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 || updated[0].Expected != 248 {
		t.Fatalf("expected A100 gpu_burn calibrated to 248, got %+v", updated)
	}
	if b := m.baselineFor("NVIDIA A100 40GB", TestGPUBurn); b.Source != "calibrated" {
		t.Fatalf("calibrated baseline should take precedence, got %+v", b)
	}
}
