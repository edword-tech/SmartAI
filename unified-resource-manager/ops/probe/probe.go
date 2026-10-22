/*
 * Unified Resource Manager - GPU / NPU commissioning probe
 *
 * 调测保障：每类资源预留 1 张卡给调测，业务任务不可占用。
 * Probe 走 发现可见 → 健康 → 分配 → 记账核对 → 释放，保证 GPU/NPU 随时可调测。
 */

package probe

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"flexai/unified-resource-manager/core/pool"
)

// Status 单项结论
type Status string

const (
	StatusPass Status = "pass"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

// Check 单步检查
type Check struct {
	Name    string `json:"name"`
	Status  Status `json:"status"`
	Detail  string `json:"detail,omitempty"`
	Elapsed string `json:"elapsed,omitempty"`
}

// Result 一类资源的调测结果
type Result struct {
	ResourceType pool.ResourceType `json:"resource_type"`
	Status       Status            `json:"status"`
	Ready        bool              `json:"ready"`
	ReservedID   string            `json:"reserved_id,omitempty"`
	AllocatedID  string            `json:"allocated_id,omitempty"`
	NodeID       string            `json:"node_id,omitempty"`
	Model        string            `json:"model,omitempty"`
	Checks       []Check           `json:"checks"`
	Error        string            `json:"error,omitempty"`
	LatencyMS    float64           `json:"latency_ms"`
}

// Readiness 一类资源的就绪视图
type Readiness struct {
	ResourceType  pool.ResourceType `json:"resource_type"`
	Ready         bool              `json:"ready"`
	Total         int               `json:"total"`
	Available     int               `json:"available"`
	Reserved      int               `json:"debug_reserved"`
	ReservedReady int               `json:"debug_reserved_ready"`
	Message       string            `json:"message,omitempty"`
}

// Report 一次调测报告
type Report struct {
	ID          string    `json:"id"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
	Status      Status    `json:"status"`
	Results     []Result  `json:"results"`
	Summary     string    `json:"summary"`
	LiveSMI     bool      `json:"live_smi"`
}

// Manager 调测管理
type Manager struct {
	resources pool.ResourcePool
	now       func() time.Time
	run       func(ctx context.Context, name string, args ...string) ([]byte, error)

	mu   sync.RWMutex
	last *Report
}

// New 创建调测管理器
func New(resources pool.ResourcePool) *Manager {
	return &Manager{
		resources: resources,
		now:       time.Now,
		run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		},
	}
}

// Last 最近一次调测
func (m *Manager) Last() *Report {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.last
}

// Readiness 当前 GPU/NPU 调测就绪状态
func (m *Manager) Readiness(ctx context.Context) []Readiness {
	types := []pool.ResourceType{pool.ResourceTypeGPU, pool.ResourceTypeNPU}
	out := make([]Readiness, 0, len(types))
	for _, t := range types {
		st := Readiness{ResourceType: t}
		rs, _ := m.resources.ListResources(ctx, map[string]string{"type": string(t)})
		for _, r := range rs {
			st.Total++
			if r.State == pool.StateAvailable && r.Health != pool.DeviceHealthFailed && !r.Cordoned {
				st.Available++
			}
			if pool.ReservedForDebug(r) {
				st.Reserved++
				if r.Health != pool.DeviceHealthFailed && !r.Cordoned {
					st.ReservedReady++
				}
			}
		}
		switch {
		case st.Total == 0:
			st.Message = "账本中无此类型设备"
		case st.ReservedReady > 0:
			st.Ready = true
			st.Message = fmt.Sprintf("调测预留 %d 张可用", st.ReservedReady)
		case st.Available > 0:
			st.Ready = true
			st.Message = "无预留卡，将使用空闲业务卡"
		default:
			st.Message = "无可用设备（可检查隔离/故障）"
		}
		out = append(out, st)
	}
	return out
}

// ReserveCapacity 每类资源预留 1 张空闲卡给调测；已有预留则跳过
func ReserveCapacity(ctx context.Context, resources *pool.UnifiedResourcePool) ([]string, error) {
	rs, err := resources.ListResources(ctx, nil)
	if err != nil {
		return nil, err
	}
	have := map[pool.ResourceType]bool{}
	for _, r := range rs {
		if pool.ReservedForDebug(r) {
			have[r.Type] = true
		}
	}
	pref := map[pool.ResourceType]string{
		pool.ResourceTypeGPU: "node-gpu-1",
		pool.ResourceTypeNPU: "node-npu-1",
	}
	sort.Slice(rs, func(i, j int) bool {
		if rs[i].NodeID != rs[j].NodeID {
			return rs[i].NodeID < rs[j].NodeID
		}
		return rs[i].DeviceID > rs[j].DeviceID // 同类节点上取编号较大的卡
	})
	var reserved []string
	for _, r := range rs {
		if have[r.Type] || r.Health == pool.DeviceHealthFailed || r.Cordoned || r.Holders > 0 {
			continue
		}
		if want, ok := pref[r.Type]; ok && r.NodeID != want {
			continue
		}
		if err := resources.SetDeviceLabel(r.ID, pool.LabelPurpose, pool.PurposeDebug); err != nil {
			return reserved, err
		}
		have[r.Type] = true
		reserved = append(reserved, r.ID)
	}
	// 首选节点没有空闲卡时，退化为任意空闲卡
	for _, r := range rs {
		if have[r.Type] || r.Health == pool.DeviceHealthFailed || r.Cordoned || r.Holders > 0 {
			continue
		}
		if err := resources.SetDeviceLabel(r.ID, pool.LabelPurpose, pool.PurposeDebug); err != nil {
			return reserved, err
		}
		have[r.Type] = true
		reserved = append(reserved, r.ID)
	}
	return reserved, nil
}

// Probe 对指定类型执行调测；types 为空则 GPU+NPU 都测
func (m *Manager) Probe(ctx context.Context, types []pool.ResourceType) *Report {
	if len(types) == 0 {
		types = []pool.ResourceType{pool.ResourceTypeGPU, pool.ResourceTypeNPU}
	}
	rep := &Report{ID: fmt.Sprintf("probe-%d", m.now().UnixNano()), StartedAt: m.now()}
	for _, t := range types {
		rep.Results = append(rep.Results, m.probeType(ctx, t))
	}
	rep.FinishedAt = m.now()
	pass, fail := 0, 0
	for _, r := range rep.Results {
		if r.Status == StatusPass {
			pass++
		} else {
			fail++
		}
	}
	if fail == 0 {
		rep.Status = StatusPass
		rep.Summary = fmt.Sprintf("GPU/NPU 调测全部通过（%d 项）", pass)
	} else {
		rep.Status = StatusFail
		rep.Summary = fmt.Sprintf("调测失败 %d / 通过 %d", fail, pass)
	}
	m.mu.Lock()
	m.last = rep
	m.mu.Unlock()
	return rep
}

func (m *Manager) probeType(ctx context.Context, rtype pool.ResourceType) Result {
	start := time.Now()
	res := Result{ResourceType: rtype, Status: StatusFail}
	add := func(name string, ok bool, detail string) {
		st := StatusPass
		if !ok {
			st = StatusFail
			if res.Error == "" {
				res.Error = name + ": " + detail
			}
		}
		res.Checks = append(res.Checks, Check{Name: name, Status: st, Detail: detail})
	}

	list, err := m.resources.ListResources(ctx, map[string]string{"type": string(rtype)})
	if err != nil || len(list) == 0 {
		add("inventory", false, fmt.Sprintf("账本中无 %s 设备: %v", rtype, err))
		res.LatencyMS = elapsedMS(start)
		return res
	}
	add("inventory", true, fmt.Sprintf("发现 %d 张 %s", len(list), rtype))

	var reserved *pool.Resource
	healthy := 0
	for _, r := range list {
		if r.Health != pool.DeviceHealthFailed && !r.Cordoned {
			healthy++
		}
		if pool.ReservedForDebug(r) {
			reserved = r
		}
	}
	add("health", healthy > 0, fmt.Sprintf("健康可调度 %d/%d", healthy, len(list)))
	if reserved != nil {
		res.ReservedID = reserved.ID
		ok := reserved.Health != pool.DeviceHealthFailed && !reserved.Cordoned
		add("debug_reserve", ok, reserved.ID+" "+reserved.Model)
	} else {
		add("debug_reserve", true, "未预留，将使用空闲卡")
	}

	req := &pool.AllocationRequest{
		TaskID:       "probe-" + string(rtype),
		TaskType:     "debug",
		ResourceType: rtype,
		Count:        1,
		Labels:       map[string]string{pool.LabelPurpose: pool.PurposeDebug},
	}
	if reserved != nil && reserved.Health != pool.DeviceHealthFailed && !reserved.Cordoned {
		req.ResourceIDs = []string{reserved.ID}
	}
	alloc, err := m.resources.AllocateResources(ctx, req)
	if err != nil {
		add("allocate", false, err.Error())
		res.LatencyMS = elapsedMS(start)
		return res
	}
	add("allocate", true, alloc.AllocationID)

	if len(alloc.Slices) == 0 {
		add("bind", false, "allocation returned no slices")
	} else {
		sl := alloc.Slices[0]
		res.AllocatedID, res.NodeID = sl.ResourceID, sl.NodeID
		got, gerr := m.resources.GetResource(ctx, sl.ResourceID)
		if gerr != nil || got.Holders < 1 {
			holders := 0
			if got != nil {
				holders = got.Holders
			}
			add("bind", false, fmt.Sprintf("账本未记账: %v holders=%d", gerr, holders))
		} else {
			res.Model = got.Model
			add("bind", true, fmt.Sprintf("%s holders=%d mode=%s", sl.ResourceID, got.Holders, alloc.ShareMode))
			if live := m.liveSMI(ctx, rtype, got.DeviceID); live.Name != "" {
				res.Checks = append(res.Checks, live)
			}
		}
	}

	if err := m.resources.ReleaseResources(ctx, alloc.AllocationID); err != nil {
		add("release", false, err.Error())
	} else {
		add("release", true, "allocation released")
	}
	if res.Error == "" {
		res.Status = StatusPass
		res.Ready = true
	}
	res.LatencyMS = elapsedMS(start)
	return res
}

func (m *Manager) liveSMI(ctx context.Context, rtype pool.ResourceType, deviceID string) Check {
	var name string
	var args []string
	switch rtype {
	case pool.ResourceTypeGPU:
		name, args = "nvidia-smi", []string{"-i", deviceID, "--query-gpu=name,uuid,memory.total", "--format=csv,noheader"}
	case pool.ResourceTypeNPU:
		name, args = "npu-smi", []string{"info", "-i", deviceID}
	default:
		return Check{}
	}
	if _, err := exec.LookPath(name); err != nil {
		return Check{Name: "live_smi", Status: StatusSkip, Detail: name + " 不在管理面 PATH，跳过本机实卡探测"}
	}
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out, err := m.run(cctx, name, args...)
	if err != nil {
		// 管理面无实卡/无驱动时不阻断平台调测，仅记录
		return Check{Name: "live_smi", Status: StatusSkip, Detail: "本机 " + name + " 不可用（" + firstLine(string(out)+" "+err.Error()) + "），账本分配已通过"}
	}
	return Check{Name: "live_smi", Status: StatusPass, Detail: firstLine(string(out))}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func elapsedMS(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000
}
