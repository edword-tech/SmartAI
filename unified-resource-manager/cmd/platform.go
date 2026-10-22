package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"flexai/unified-resource-manager/adapters/gpu"
	"flexai/unified-resource-manager/adapters/npu"
	"flexai/unified-resource-manager/core/cluster"
	"flexai/unified-resource-manager/core/pool"
	"flexai/unified-resource-manager/core/scheduler"
	"flexai/unified-resource-manager/ops/benchmark"
	"flexai/unified-resource-manager/ops/diagnosis"
	"flexai/unified-resource-manager/ops/remediation"
	"flexai/unified-resource-manager/ops/telemetry"
)

// platformActuator 把自愈动作落到账本 + 调度器
type platformActuator struct {
	pool  *pool.UnifiedResourcePool
	sched *scheduler.IntelligentScheduler
}

func (a *platformActuator) CordonDevice(id string, c bool) error {
	return a.pool.CordonDevice(id, c)
}
func (a *platformActuator) SetDeviceHealth(id, h string) error {
	return a.pool.SetDeviceHealth(id, h)
}
func (a *platformActuator) RescheduleDevice(id, reason string) []string {
	return a.sched.HandleResourceFailure([]string{id}, reason)
}
func (a *platformActuator) RescheduleNode(nodeID, reason string) []string {
	return a.sched.HandleNodeFailure(nodeID, reason)
}

// platformInventory 压测用的节点资产视图
type platformInventory struct {
	cluster *cluster.ClusterManager
	pool    *pool.UnifiedResourcePool
}

func (p *platformInventory) NodeDevices(nodeID string) []*pool.Resource {
	rs, err := p.pool.ListResources(context.Background(), map[string]string{"node_id": nodeID})
	if err != nil {
		return nil
	}
	return rs
}

func (p *platformInventory) NodeNICs(nodeID string) []benchmark.NIC {
	n, err := p.cluster.GetNode(nodeID)
	if err != nil || n.Network == nil {
		return nil
	}
	out := make([]benchmark.NIC, 0, len(n.Network.RDMANics))
	for _, nic := range n.Network.RDMANics {
		out = append(out, benchmark.NIC{Name: nic.Name, RateGbps: nic.RateGbps, State: nic.State})
	}
	return out
}

func syncNodeToLedgers(node *cluster.Node, gpuAd *gpu.GPUAdapter, npuAd *npu.NPUAdapter) {
	if node == nil || node.Resources == nil {
		return
	}
	if len(node.Resources.GPUs) > 0 {
		gpuAd.SyncNode(node.ID, node.Resources.GPUSpecs())
	}
	if len(node.Resources.NPUs) > 0 {
		npuAd.SyncNode(node.ID, node.Resources.NPUSpecs())
	}
}

func wireClusterOps(clusterMgr *cluster.ClusterManager, gpuAd *gpu.GPUAdapter, npuAd *npu.NPUAdapter, engine *remediation.Engine) {
	clusterMgr.AddListener(func(node *cluster.Node, event cluster.NodeEvent) {
		switch event {
		case cluster.NodeEventRegistered, cluster.NodeEventResourcesUpdated:
			syncNodeToLedgers(node, gpuAd, npuAd)
		case cluster.NodeEventFailed:
			if engine != nil {
				engine.HandleNodeFailure(node.ID, "heartbeat lost / node error")
			}
		case cluster.NodeEventRecovered:
			if engine != nil {
				engine.HandleNodeRecovered(node.ID)
			}
		}
	})
}

func newTelemetryHub(resourcePool *pool.UnifiedResourcePool, gpuExporter string) (*telemetry.Hub, *telemetry.SimulatedSource) {
	sim := telemetry.NewSimulatedSource()
	var primary telemetry.Source = sim
	if url := firstNonEmpty(os.Getenv("URM_PROMETHEUS_URL"), gpuExporter); url != "" && looksLikeHTTP(url) {
		if os.Getenv("URM_TELEMETRY") == "prometheus" {
			primary = telemetry.NewPrometheusSource(url)
			log.Printf("Telemetry primary source: Prometheus at %s", url)
		}
	}
	hub := telemetry.NewHub(resourcePool, primary, sim, 0)
	return hub, sim
}

func newDiagnoser() *diagnosis.Diagnoser {
	return diagnosis.New(diagnosis.LLMConfigFromEnv())
}

func initSchedulerQueues(sched *scheduler.IntelligentScheduler) {
	_ = sched.Queues().Upsert(&scheduler.Queue{
		Name: "team1", TenantID: "tenant-team1", Weight: 2, Reclaimable: true,
		Guaranteed: scheduler.ResourceList{GPU: 4, NPU: 2, MemoryGB: 160},
		Capability: scheduler.ResourceList{GPU: 20, NPU: 10, MemoryGB: 2048},
	})
	_ = sched.Queues().Upsert(&scheduler.Queue{
		Name: "team2", TenantID: "tenant-team2", Weight: 1, Reclaimable: true,
		Guaranteed: scheduler.ResourceList{GPU: 2, NPU: 1, MemoryGB: 80},
		Capability: scheduler.ResourceList{GPU: 10, NPU: 5, MemoryGB: 1024},
	})
	_ = sched.Queues().Upsert(&scheduler.Queue{
		Name: "training", Weight: 3, Reclaimable: false,
		Guaranteed: scheduler.ResourceList{GPU: 8, MemoryGB: 640},
	})
	_ = sched.Queues().Upsert(&scheduler.Queue{
		Name: "debug", Weight: 5, Reclaimable: false,
		Guaranteed: scheduler.ResourceList{GPU: 1, NPU: 1, MemoryGB: 80},
	})
}

func gpuSpecs(model string, count int, mem int64, nvlPrefix string) []*cluster.GPUInfo {
	out := make([]*cluster.GPUInfo, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, &cluster.GPUInfo{
			DeviceID: strconv.Itoa(i), Model: model, TotalMemory: mem, TotalCores: 100,
			Topology: &pool.DeviceTopology{
				NUMANode:    i / 4,
				PCIeSwitch:  "sw" + strconv.Itoa(i/2),
				NVLinkGroup: nvlPrefix + strconv.Itoa(i/4),
				RDMANic:     "mlx5_" + strconv.Itoa(i/4),
			},
		})
	}
	return out
}

func npuSpecs(model string, count int, mem int64, cores int, hccsPrefix string) []*cluster.NPUInfo {
	out := make([]*cluster.NPUInfo, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, &cluster.NPUInfo{
			DeviceID: strconv.Itoa(i), Model: model, TotalMemory: mem, TotalCores: cores,
			Topology: &pool.DeviceTopology{
				NUMANode:    i / 4,
				NVLinkGroup: hccsPrefix + strconv.Itoa(i/4),
				RDMANic:     "hns_" + strconv.Itoa(i/4),
			},
		})
	}
	return out
}

func rdmaNet(domain string, rate int, nics int) *cluster.NodeNetwork {
	net := &cluster.NodeNetwork{RDMADomain: domain}
	for i := 0; i < nics; i++ {
		net.RDMANics = append(net.RDMANics, &cluster.RDMANic{
			Name: "mlx5_" + strconv.Itoa(i), RateGbps: rate, State: "active", LinkType: "InfiniBand",
		})
	}
	return net
}

func looksLikeHTTP(s string) bool {
	return len(s) > 7 && (s[:7] == "http://" || (len(s) > 8 && s[:8] == "https://"))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
