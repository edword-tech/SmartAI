/*
 * Unified Resource Manager - Benchmark / Baseline / Acceptance
 *
 * 集群交付验收与性能基线：
 * - 压测项：gpu_burn（单卡算力）、nccl_allreduce（机内集合通信带宽）、rdma_bw（RDMA 网卡带宽）、dcgm_diag（硬件诊断）
 * - 基线：按型号维护期望值，支持用一次通过的实测结果校准（建立本集群基线）
 * - 异常归因：结合遥测（温度/功耗/ECC/XID/NVLink/PCIe）与同节点其它结果定位根因
 * - 验收：节点全部压测项通过才标记 acceptance=passed，否则隔离不入池
 * 结果来源：内置仿真执行器，或 scripts/ops 下脚本在真实节点上执行后回传（Ingest）
 */

package benchmark

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"flexai/unified-resource-manager/core/pool"
	"flexai/unified-resource-manager/ops/telemetry"
)

// TestType 压测项
type TestType string

const (
	TestGPUBurn       TestType = "gpu_burn"
	TestNCCLAllReduce TestType = "nccl_allreduce"
	TestRDMABandwidth TestType = "rdma_bw"
	TestDCGMDiag      TestType = "dcgm_diag"
)

// AllTests 验收默认执行的全部压测项
var AllTests = []TestType{TestDCGMDiag, TestGPUBurn, TestNCCLAllReduce, TestRDMABandwidth}

// Status 结果状态
type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

// Baseline 性能基线
type Baseline struct {
	Model    string   `json:"model"` // 型号关键字（大小写不敏感的包含匹配），"*" 为兜底
	Test     TestType `json:"test"`
	Metric   string   `json:"metric"`
	Unit     string   `json:"unit"`
	Expected float64  `json:"expected"`
	// 实测/期望 低于 WarnRatio 告警，低于 FailRatio 判定失败
	WarnRatio  float64   `json:"warn_ratio"`
	FailRatio  float64   `json:"fail_ratio"`
	Source     string    `json:"source"` // default / calibrated / manual
	UpdatedAt  time.Time `json:"updated_at"`
	SampleSize int       `json:"sample_size,omitempty"`
}

// DefaultBaselines 默认参考基线（经验值，交付时应使用 Calibrate 以本集群实测校准）
func DefaultBaselines() []Baseline {
	b := func(model string, test TestType, metric, unit string, exp float64) Baseline {
		return Baseline{Model: model, Test: test, Metric: metric, Unit: unit, Expected: exp, WarnRatio: 0.92, FailRatio: 0.85, Source: "default"}
	}
	return []Baseline{
		b("H100", TestGPUBurn, "fp16_tflops", "TFLOPS", 600),
		b("H800", TestGPUBurn, "fp16_tflops", "TFLOPS", 600),
		b("H20", TestGPUBurn, "fp16_tflops", "TFLOPS", 120),
		b("A100", TestGPUBurn, "fp16_tflops", "TFLOPS", 230),
		b("A800", TestGPUBurn, "fp16_tflops", "TFLOPS", 230),
		b("L40S", TestGPUBurn, "fp16_tflops", "TFLOPS", 280),
		b("4090", TestGPUBurn, "fp16_tflops", "TFLOPS", 140),
		b("910B", TestGPUBurn, "fp16_tflops", "TFLOPS", 260),
		b("*", TestGPUBurn, "fp16_tflops", "TFLOPS", 100),

		b("H100", TestNCCLAllReduce, "busbw", "GB/s", 450),
		b("H800", TestNCCLAllReduce, "busbw", "GB/s", 180),
		b("H20", TestNCCLAllReduce, "busbw", "GB/s", 400),
		b("A100", TestNCCLAllReduce, "busbw", "GB/s", 230),
		b("A800", TestNCCLAllReduce, "busbw", "GB/s", 160),
		b("910B", TestNCCLAllReduce, "busbw", "GB/s", 170),
		b("PCIE", TestNCCLAllReduce, "busbw", "GB/s", 20),
		b("*", TestNCCLAllReduce, "busbw", "GB/s", 20),

		// RDMA 期望值为线速比例，实际期望 = 网卡速率 × Expected
		b("*", TestRDMABandwidth, "line_rate_ratio", "Gbps", 0.92),
		{Model: "*", Test: TestDCGMDiag, Metric: "passed", Unit: "bool", Expected: 1, WarnRatio: 1, FailRatio: 1, Source: "default"},
	}
}

// NIC 节点 RDMA 网卡
type NIC struct {
	Name     string `json:"name"`
	RateGbps int    `json:"rate_gbps"`
	State    string `json:"state"`
}

// Inventory 节点资产视图
type Inventory interface {
	NodeDevices(nodeID string) []*pool.Resource
	NodeNICs(nodeID string) []NIC
}

// TelemetryLookup 设备遥测查询
type TelemetryLookup interface {
	Get(resourceID string) (*telemetry.DeviceTelemetry, bool)
}

// FaultLookup 仿真故障查询（可选）
type FaultLookup interface {
	Fault(resourceID string) (telemetry.Fault, bool)
}

// Result 单项结果
type Result struct {
	ID          string             `json:"id"`
	RunID       string             `json:"run_id"`
	NodeID      string             `json:"node_id"`
	Target      string             `json:"target"` // 设备 ID / 网卡名 / node
	ResourceID  string             `json:"resource_id,omitempty"`
	Model       string             `json:"model,omitempty"`
	Test        TestType           `json:"test"`
	Metric      string             `json:"metric"`
	Unit        string             `json:"unit"`
	Value       float64            `json:"value"`
	Expected    float64            `json:"expected"`
	Ratio       float64            `json:"ratio"`
	Status      Status             `json:"status"`
	Attribution string             `json:"attribution,omitempty"`
	Evidence    map[string]float64 `json:"evidence,omitempty"`
	Source      string             `json:"source"`
}

// Run 一次压测/验收
type Run struct {
	ID         string     `json:"id"`
	NodeID     string     `json:"node_id"`
	Kind       string     `json:"kind"` // benchmark / acceptance
	Tests      []TestType `json:"tests"`
	Source     string     `json:"source"` // simulated / agent
	Status     string     `json:"status"`
	Verdict    Status     `json:"verdict"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt time.Time  `json:"finished_at"`
	Results    []Result   `json:"results"`
	Summary    []string   `json:"summary"`
}

// AcceptanceHook 验收结论回调（用于设置节点标签 / 隔离）
type AcceptanceHook func(nodeID string, passed bool, run *Run)

// Manager 压测与验收管理
type Manager struct {
	inventory Inventory
	telemetry TelemetryLookup
	faults    FaultLookup
	now       func() time.Time

	baselines []Baseline
	runs      []*Run
	maxRuns   int
	hooks     []AcceptanceHook
	mu        sync.RWMutex
}

// NewManager 创建管理器；faults 可为 nil
func NewManager(inv Inventory, tele TelemetryLookup, faults FaultLookup) *Manager {
	return &Manager{inventory: inv, telemetry: tele, faults: faults, now: time.Now, baselines: DefaultBaselines(), maxRuns: 200}
}

// OnAcceptance 注册验收回调
func (m *Manager) OnAcceptance(h AcceptanceHook) {
	m.mu.Lock()
	m.hooks = append(m.hooks, h)
	m.mu.Unlock()
}

// Baselines 返回基线
func (m *Manager) Baselines() []Baseline {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]Baseline(nil), m.baselines...)
}

// SetBaseline 新增或覆盖基线
func (m *Manager) SetBaseline(b Baseline) error {
	if b.Model == "" || b.Test == "" || b.Expected <= 0 {
		return fmt.Errorf("model, test and positive expected are required")
	}
	if b.WarnRatio <= 0 {
		b.WarnRatio = 0.92
	}
	if b.FailRatio <= 0 {
		b.FailRatio = 0.85
	}
	if b.Source == "" {
		b.Source = "manual"
	}
	b.UpdatedAt = m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, old := range m.baselines {
		if strings.EqualFold(old.Model, b.Model) && old.Test == b.Test {
			if b.Metric == "" {
				b.Metric, b.Unit = old.Metric, old.Unit
			}
			m.baselines[i] = b
			return nil
		}
	}
	m.baselines = append(m.baselines, b)
	return nil
}

func (m *Manager) baselineFor(model string, test TestType) Baseline {
	m.mu.RLock()
	defer m.mu.RUnlock()
	up := strings.ToUpper(model)
	// 取匹配最精确（关键字最长）的基线，校准后的完整型号优先于默认关键字
	var best, fallback *Baseline
	for i := range m.baselines {
		b := &m.baselines[i]
		if b.Test != test {
			continue
		}
		if b.Model == "*" {
			fallback = b
			continue
		}
		if strings.Contains(up, strings.ToUpper(b.Model)) && (best == nil || len(b.Model) > len(best.Model)) {
			best = b
		}
	}
	if best != nil {
		return *best
	}
	if fallback != nil {
		return *fallback
	}
	return Baseline{Model: "*", Test: test, Expected: 1, WarnRatio: 0.92, FailRatio: 0.85}
}

// Runs 列出运行记录（新的在前）
func (m *Manager) Runs(nodeID string) []*Run {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []*Run{}
	for i := len(m.runs) - 1; i >= 0; i-- {
		if nodeID == "" || m.runs[i].NodeID == nodeID {
			out = append(out, m.runs[i])
		}
	}
	return out
}

// GetRun 获取运行记录
func (m *Manager) GetRun(id string) (*Run, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.runs {
		if r.ID == id {
			return r, true
		}
	}
	return nil, false
}

func noise(key string, amplitude float64) float64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return 1 + amplitude*(float64(h.Sum32()%2001)/1000-1)
}

func (m *Manager) fault(id string) telemetry.Fault {
	if m.faults == nil {
		return telemetry.Fault{}
	}
	f, _ := m.faults.Fault(id)
	return f
}

func (m *Manager) tele(id string) *telemetry.DeviceTelemetry {
	if m.telemetry == nil {
		return nil
	}
	t, _ := m.telemetry.Get(id)
	return t
}

// simulate 仿真执行器：以基线为中心加入抖动，并叠加注入故障 / 遥测异常的影响
func (m *Manager) simulate(runID, nodeID string, tests []TestType) []Result {
	devices := m.inventory.NodeDevices(nodeID)
	var out []Result
	for _, test := range tests {
		switch test {
		case TestDCGMDiag:
			for _, d := range devices {
				v := 1.0
				t := m.tele(d.ID)
				if t != nil && (t.ECCDoubleBit > 0 || FatalXID(t.XID)) {
					v = 0
				}
				out = append(out, m.newResult(runID, nodeID, d, test, v, "simulated"))
			}
		case TestGPUBurn:
			for _, d := range devices {
				b := m.baselineFor(d.Model, test)
				v := b.Expected * noise(runID+d.ID, 0.03)
				if f := m.fault(d.ID); f.PerfFactor > 0 {
					v *= f.PerfFactor
				}
				if t := m.tele(d.ID); t != nil {
					if t.XID == 79 {
						v = 0
					} else if t.TemperatureC >= 87 || len(t.Throttle) > 0 {
						v *= 0.78
					}
				}
				out = append(out, m.newResult(runID, nodeID, d, test, v, "simulated"))
			}
		case TestNCCLAllReduce:
			if len(devices) < 2 {
				continue
			}
			model := nodeModel(devices)
			if !sameNVLinkGroup(devices) {
				model = "PCIE"
			}
			b := m.baselineFor(model, test)
			v := b.Expected * noise(runID+nodeID+"nccl", 0.03)
			for _, d := range devices {
				if f := m.fault(d.ID); f.PerfFactor > 0 {
					v = math.Min(v, b.Expected*f.PerfFactor)
				}
				if t := m.tele(d.ID); t != nil {
					if t.XID == 79 {
						v = 0
					}
					if t.NVLinkCRCErrors > 0 {
						v = math.Min(v, b.Expected*0.55)
					}
				}
			}
			r := m.newResult(runID, nodeID, &pool.Resource{NodeID: nodeID, DeviceID: "node", Model: model}, test, v, "simulated")
			out = append(out, r)
		case TestRDMABandwidth:
			for _, nic := range m.inventory.NodeNICs(nodeID) {
				b := m.baselineFor("*", test)
				v := float64(nic.RateGbps) * b.Expected * noise(runID+nic.Name, 0.02)
				if !nicUp(nic.State) {
					v = 0
				}
				if f := m.fault(nodeID + "/" + nic.Name); f.PerfFactor > 0 {
					v *= f.PerfFactor
				}
				out = append(out, m.nicResult(runID, nodeID, nic, v, "simulated"))
			}
		}
	}
	return out
}

func nicUp(state string) bool {
	s := strings.ToLower(state)
	return s == "" || s == "up" || s == "active"
}

// FatalXID 是否致命 XID
func FatalXID(x int) bool {
	switch x {
	case 48, 63, 64, 74, 79, 92, 94, 95, 119, 120:
		return true
	}
	return false
}

func nodeModel(devices []*pool.Resource) string {
	if len(devices) == 0 {
		return ""
	}
	return devices[0].Model
}

func sameNVLinkGroup(devices []*pool.Resource) bool {
	group := ""
	for _, d := range devices {
		if d.Topology == nil || d.Topology.NVLinkGroup == "" {
			return false
		}
		if group == "" {
			group = d.Topology.NVLinkGroup
		} else if group != d.Topology.NVLinkGroup {
			return false
		}
	}
	return true
}

func (m *Manager) newResult(runID, nodeID string, d *pool.Resource, test TestType, value float64, source string) Result {
	b := m.baselineFor(d.Model, test)
	r := Result{ID: uuid.New().String(), RunID: runID, NodeID: nodeID, Target: d.DeviceID, ResourceID: d.ID, Model: d.Model,
		Test: test, Metric: b.Metric, Unit: b.Unit, Value: round(value), Expected: b.Expected, Source: source}
	grade(&r, b)
	return r
}

func (m *Manager) nicResult(runID, nodeID string, nic NIC, value float64, source string) Result {
	b := m.baselineFor("*", TestRDMABandwidth)
	r := Result{ID: uuid.New().String(), RunID: runID, NodeID: nodeID, Target: nic.Name, Test: TestRDMABandwidth,
		Metric: "bw_gbps", Unit: "Gbps", Value: round(value), Expected: round(float64(nic.RateGbps) * b.Expected), Source: source,
		Evidence: map[string]float64{"line_rate_gbps": float64(nic.RateGbps)}}
	if !nicUp(nic.State) {
		r.Evidence["link_down"] = 1
	}
	grade(&r, b)
	return r
}

func grade(r *Result, b Baseline) {
	if r.Expected <= 0 {
		r.Status = StatusPass
		return
	}
	r.Ratio = round(r.Value / r.Expected)
	switch {
	case r.Ratio < b.FailRatio:
		r.Status = StatusFail
	case r.Ratio < b.WarnRatio:
		r.Status = StatusWarn
	default:
		r.Status = StatusPass
	}
}

func round(v float64) float64 { return math.Round(v*100) / 100 }

// attribute 异常归因
func (m *Manager) attribute(results []Result) {
	burnFailed, burnTotal := map[string]int{}, map[string]int{}
	for _, r := range results {
		if r.Test == TestGPUBurn {
			burnTotal[r.NodeID]++
			if r.Status != StatusPass {
				burnFailed[r.NodeID]++
			}
		}
	}
	for i := range results {
		r := &results[i]
		if r.Status == StatusPass {
			continue
		}
		if r.Evidence == nil {
			r.Evidence = map[string]float64{}
		}
		var t *telemetry.DeviceTelemetry
		if r.ResourceID != "" {
			t = m.tele(r.ResourceID)
		}
		if t != nil {
			r.Evidence["temperature_c"] = t.TemperatureC
			r.Evidence["power_w"] = t.PowerW
			r.Evidence["power_limit_w"] = t.PowerLimitW
			r.Evidence["ecc_dbe"] = float64(t.ECCDoubleBit)
			r.Evidence["xid"] = float64(t.XID)
		}
		switch r.Test {
		case TestDCGMDiag:
			switch {
			case t != nil && t.ECCDoubleBit > 0:
				r.Attribution = "显存 ECC 双比特错误：建议 reset 后复测，仍复现则返修"
			case t != nil && FatalXID(t.XID):
				r.Attribution = fmt.Sprintf("致命 XID %d：检查驱动日志 (dmesg)、PCIe 链路与供电", t.XID)
			default:
				r.Attribution = "硬件诊断未通过：执行 dcgmi diag -r 3 获取详细失败项"
			}
		case TestGPUBurn:
			switch {
			case r.Value == 0 || (t != nil && t.XID == 79):
				r.Attribution = "设备掉卡（XID 79）：检查 PCIe 插槽/Riser、供电与 GPU 本体"
			case t != nil && (t.TemperatureC >= 85 || len(t.Throttle) > 0):
				r.Attribution = fmt.Sprintf("温度墙降频（%.0f°C）：检查风扇/风道/液冷流量与机房进风温度", t.TemperatureC)
			case t != nil && t.PowerLimitW > 0 && t.PowerW >= 0.95*t.PowerLimitW:
				r.Attribution = "功耗墙限制：检查电源冗余与 nvidia-smi -pl 功耗上限设置"
			case burnTotal[r.NodeID] > 1 && burnFailed[r.NodeID] == burnTotal[r.NodeID]:
				r.Attribution = "节点级性能偏低（全部卡）：检查驱动/固件版本一致性、BIOS 性能模式、CPU 频率策略"
			default:
				r.Attribution = "单卡算力低于同型号基线：与同节点其它卡对比，执行 dcgmi diag -r 3 排查硬件"
			}
		case TestNCCLAllReduce:
			nvlinkErr, replay := false, false
			for _, d := range m.inventory.NodeDevices(r.NodeID) {
				if dt := m.tele(d.ID); dt != nil {
					nvlinkErr = nvlinkErr || dt.NVLinkCRCErrors > 0
					replay = replay || dt.PCIeReplay > 0
				}
			}
			switch {
			case burnFailed[r.NodeID] > 0:
				r.Attribution = "受单卡算力/掉卡异常拖累：先处理 gpu_burn 失败的设备"
			case nvlinkErr:
				r.Attribution = "NVLink 链路 CRC 错误导致带宽下降：检查 NVSwitch/NVLink 桥接，nvidia-smi nvlink -e"
			case replay:
				r.Attribution = "PCIe 链路重传：检查 PCIe 代际/宽度协商 (lspci -vv) 与 Riser"
			default:
				r.Attribution = "通信配置问题：检查 nvidia-smi topo -m 拓扑、NCCL_P2P_LEVEL / NCCL_IB_HCA 环境变量、ACS/IOMMU 设置"
			}
		case TestRDMABandwidth:
			if r.Evidence["link_down"] == 1 {
				r.Attribution = "RDMA 网卡链路 down：检查光模块/线缆/交换机端口 (ibstat / ethtool)"
			} else {
				r.Attribution = "RDMA 带宽低于线速：检查 RoCE PFC/ECN、MTU、GID index、网卡所在 PCIe 插槽带宽与 NUMA 亲和"
			}
		}
	}
}

func (m *Manager) finish(run *Run) *Run {
	m.attribute(run.Results)
	run.Verdict = StatusPass
	counts := map[Status]int{}
	for _, r := range run.Results {
		counts[r.Status]++
		if r.Status == StatusFail {
			run.Verdict = StatusFail
		} else if r.Status == StatusWarn && run.Verdict == StatusPass {
			run.Verdict = StatusWarn
		}
	}
	run.Status = "completed"
	run.FinishedAt = m.now()
	run.Summary = []string{fmt.Sprintf("共 %d 项：通过 %d，告警 %d，失败 %d", len(run.Results), counts[StatusPass], counts[StatusWarn], counts[StatusFail])}
	seen := map[string]bool{}
	for _, r := range run.Results {
		if r.Attribution != "" && !seen[r.Attribution] {
			seen[r.Attribution] = true
			run.Summary = append(run.Summary, fmt.Sprintf("[%s %s] %s", r.Test, r.Target, r.Attribution))
		}
	}

	m.mu.Lock()
	m.runs = append(m.runs, run)
	if len(m.runs) > m.maxRuns {
		m.runs = m.runs[len(m.runs)-m.maxRuns:]
	}
	hooks := append([]AcceptanceHook(nil), m.hooks...)
	m.mu.Unlock()

	if run.Kind == "acceptance" {
		// 验收以无失败项为准，告警项允许入池
		passed := run.Verdict != StatusFail
		for _, h := range hooks {
			h(run.NodeID, passed, run)
		}
	}
	return run
}

// RunSimulated 在仿真执行器上运行压测
func (m *Manager) RunSimulated(nodeID, kind string, tests []TestType) (*Run, error) {
	if len(m.inventory.NodeDevices(nodeID)) == 0 && len(m.inventory.NodeNICs(nodeID)) == 0 {
		return nil, fmt.Errorf("node %s has no devices or NICs", nodeID)
	}
	if len(tests) == 0 {
		tests = AllTests
	}
	if kind == "" {
		kind = "benchmark"
	}
	run := &Run{ID: uuid.New().String(), NodeID: nodeID, Kind: kind, Tests: tests, Source: "simulated", Status: "running", StartedAt: m.now()}
	run.Results = m.simulate(run.ID, nodeID, tests)
	return m.finish(run), nil
}

// IngestInput 脚本回传的单项结果
type IngestInput struct {
	Test     TestType           `json:"test"`
	Target   string             `json:"target"` // 设备 ID（如 0）、网卡名或 node
	Value    float64            `json:"value"`
	Evidence map[string]float64 `json:"evidence,omitempty"`
}

// Ingest 接收真实节点上脚本执行的结果，统一按基线评级与归因
func (m *Manager) Ingest(nodeID, kind string, inputs []IngestInput) (*Run, error) {
	if nodeID == "" || len(inputs) == 0 {
		return nil, fmt.Errorf("node_id and results are required")
	}
	if kind == "" {
		kind = "benchmark"
	}
	devices := map[string]*pool.Resource{}
	for _, d := range m.inventory.NodeDevices(nodeID) {
		devices[d.DeviceID] = d
	}
	nics := map[string]NIC{}
	for _, n := range m.inventory.NodeNICs(nodeID) {
		nics[n.Name] = n
	}
	run := &Run{ID: uuid.New().String(), NodeID: nodeID, Kind: kind, Source: "agent", Status: "running", StartedAt: m.now()}
	testSet := map[TestType]bool{}
	for _, in := range inputs {
		testSet[in.Test] = true
		var r Result
		switch in.Test {
		case TestRDMABandwidth:
			nic, ok := nics[in.Target]
			if !ok {
				nic = NIC{Name: in.Target, RateGbps: int(in.Evidence["line_rate_gbps"]), State: "up"}
			}
			r = m.nicResult(run.ID, nodeID, nic, in.Value, "agent")
		case TestGPUBurn, TestDCGMDiag, TestNCCLAllReduce:
			d, ok := devices[in.Target]
			if !ok {
				all := m.inventory.NodeDevices(nodeID)
				model := nodeModel(all)
				if in.Test == TestNCCLAllReduce && !sameNVLinkGroup(all) {
					model = "PCIE"
				}
				d = &pool.Resource{NodeID: nodeID, DeviceID: in.Target, Model: model}
				if in.Test != TestNCCLAllReduce {
					d.ID = pool.MakeResourceID(nodeID, in.Target)
				}
			}
			r = m.newResult(run.ID, nodeID, d, in.Test, in.Value, "agent")
		default:
			return nil, fmt.Errorf("unknown test %q", in.Test)
		}
		for k, v := range in.Evidence {
			if r.Evidence == nil {
				r.Evidence = map[string]float64{}
			}
			r.Evidence[k] = v
		}
		run.Results = append(run.Results, r)
	}
	for t := range testSet {
		run.Tests = append(run.Tests, t)
	}
	sort.Slice(run.Tests, func(i, j int) bool { return run.Tests[i] < run.Tests[j] })
	return m.finish(run), nil
}

// Calibrate 使用指定运行记录中通过的结果，按型号取中位数更新基线
func (m *Manager) Calibrate(runID string) ([]Baseline, error) {
	run, ok := m.GetRun(runID)
	if !ok {
		return nil, fmt.Errorf("run %s not found", runID)
	}
	samples := map[string][]float64{}
	keyInfo := map[string]Result{}
	for _, r := range run.Results {
		if r.Status == StatusFail || r.Test == TestDCGMDiag || r.Test == TestRDMABandwidth || r.Model == "" {
			continue
		}
		k := r.Model + "|" + string(r.Test)
		samples[k] = append(samples[k], r.Value)
		keyInfo[k] = r
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("run %s has no calibratable results", runID)
	}
	var updated []Baseline
	for k, vals := range samples {
		sort.Float64s(vals)
		median := vals[len(vals)/2]
		if len(vals)%2 == 0 {
			median = (vals[len(vals)/2-1] + vals[len(vals)/2]) / 2
		}
		info := keyInfo[k]
		old := m.baselineFor(info.Model, info.Test)
		b := Baseline{Model: info.Model, Test: info.Test, Metric: info.Metric, Unit: info.Unit, Expected: round(median),
			WarnRatio: old.WarnRatio, FailRatio: old.FailRatio, Source: "calibrated", SampleSize: len(vals)}
		if err := m.SetBaseline(b); err != nil {
			return nil, err
		}
		updated = append(updated, b)
	}
	sort.Slice(updated, func(i, j int) bool {
		return updated[i].Model+string(updated[i].Test) < updated[j].Model+string(updated[j].Test)
	})
	return updated, nil
}
