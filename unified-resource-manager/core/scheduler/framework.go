/*
 * Unified Resource Manager - Scheduling Framework
 *
 * 参照 Kubernetes Scheduler Framework 的扩展点组织调度逻辑：
 *   QueueSort -> PreFilter(队列配额) -> Filter(节点) -> 设备落位(拓扑感知) -> Score(加权打分)
 *   -> PostFilter(抢占) -> Permit(Gang 全有或全无) -> Bind(资源池精确落位)
 * 快照 (Snapshot) 在一个调度周期内被试探性修改，Gang 与抢占均在快照上模拟后才真正提交。
 */

package scheduler

import (
	"fmt"
	"sort"
	"strings"

	"flexai/unified-resource-manager/core/pool"
)

// NodeInspector 提供节点可调度性与拓扑标签（由集群管理器实现）
type NodeInspector interface {
	IsSchedulable(nodeID string) bool
	NodeLabels(nodeID string) map[string]string
}

// 节点标签约定
const (
	LabelRDMADomain      = "topology.rdma_domain" // 同一 RDMA 域节点间通信开销最低
	LabelTenantDedicated = "tenant.dedicated"     // 节点独占给某租户，实现多租户资源隔离
	// LabelPlacementPolicy 任务级放置策略覆盖：spread / binpack / topology
	LabelPlacementPolicy = "placement_policy"
)

// 任务级放置策略
const (
	PolicySpread   = "spread"
	PolicyBinpack  = "binpack"
	PolicyTopology = "topology"
)

// NodeInfo 调度快照中的节点
type NodeInfo struct {
	Name        string
	Labels      map[string]string
	Schedulable bool
	Devices     []*pool.Resource
}

// Snapshot 调度快照
type Snapshot struct {
	Nodes          map[string]*NodeInfo
	order          []string
	byID           map[string]*pool.Resource
	CoreOvercommit float64
}

func newSnapshot(resources []*pool.Resource, inspector NodeInspector, overcommit float64) *Snapshot {
	s := &Snapshot{Nodes: make(map[string]*NodeInfo), byID: make(map[string]*pool.Resource), CoreOvercommit: overcommit}
	for _, r := range resources {
		cp := *r
		n, ok := s.Nodes[r.NodeID]
		if !ok {
			n = &NodeInfo{Name: r.NodeID, Schedulable: true, Labels: map[string]string{}}
			if inspector != nil {
				n.Schedulable = inspector.IsSchedulable(r.NodeID)
				if l := inspector.NodeLabels(r.NodeID); l != nil {
					n.Labels = l
				}
			}
			s.Nodes[r.NodeID] = n
			s.order = append(s.order, r.NodeID)
		}
		n.Devices = append(n.Devices, &cp)
		s.byID[cp.ID] = &cp
	}
	sort.Strings(s.order)
	return s
}

// Clone 深拷贝快照（设备数值字段可独立修改）
func (s *Snapshot) Clone() *Snapshot {
	c := &Snapshot{Nodes: make(map[string]*NodeInfo, len(s.Nodes)), byID: make(map[string]*pool.Resource, len(s.byID)), order: s.order, CoreOvercommit: s.CoreOvercommit}
	for name, n := range s.Nodes {
		cn := &NodeInfo{Name: n.Name, Labels: n.Labels, Schedulable: n.Schedulable}
		for _, d := range n.Devices {
			cp := *d
			cn.Devices = append(cn.Devices, &cp)
			c.byID[cp.ID] = &cp
		}
		c.Nodes[name] = cn
	}
	return c
}

// Assume 在快照上占用切片
func (s *Snapshot) Assume(slices []pool.AllocationSlice, mode pool.ShareMode) {
	for _, sl := range slices {
		d, ok := s.byID[sl.ResourceID]
		if !ok {
			continue
		}
		d.UsedMemory += sl.MemoryGB
		d.UsedCores += sl.Cores
		d.MIGSlices += sl.MIGSlices
		d.Holders++
		d.ShareMode = mode
	}
}

// Forget 在快照上释放切片
func (s *Snapshot) Forget(slices []pool.AllocationSlice) {
	for _, sl := range slices {
		d, ok := s.byID[sl.ResourceID]
		if !ok {
			continue
		}
		d.UsedMemory -= sl.MemoryGB
		d.UsedCores -= sl.Cores
		d.MIGSlices -= sl.MIGSlices
		d.Holders--
		if d.Holders <= 0 {
			d.Holders, d.UsedMemory, d.UsedCores, d.MIGSlices, d.ShareMode = 0, 0, 0, 0, ""
		}
	}
}

// Status 插件返回状态
type Status struct {
	Plugin string
	Reason string
}

func (st *Status) Error() string { return st.Plugin + ": " + st.Reason }

// CycleState 单次调度周期内的共享状态
type CycleState struct {
	// GangDomains 同一 Gang 已落位成员所在的 RDMA 域 -> 成员数
	GangDomains map[string]int
	// GangNodes 同一 Gang 已落位成员所在节点
	GangNodes map[string]int
}

// FilterPlugin 节点过滤插件
type FilterPlugin interface {
	Name() string
	Filter(task *Task, node *NodeInfo) *Status
}

// ScorePlugin 节点打分插件，返回 0-100
type ScorePlugin interface {
	Name() string
	Score(state *CycleState, task *Task, node *NodeInfo, p *Placement) int64
}

// Placement 候选落位
type Placement struct {
	Node          string
	Slices        []pool.AllocationSlice
	Mode          pool.ShareMode
	TopologyScore int64 // 设备间互联质量 0-100
	Score         int64
	ScoreDetail   map[string]int64
}

// ResourceIDs 返回落位设备 ID
func (p *Placement) ResourceIDs() []string {
	out := make([]string, 0, len(p.Slices))
	for _, s := range p.Slices {
		out = append(out, s.ResourceID)
	}
	return out
}

// ---------------- Filter plugins ----------------

// NodeSchedulableFilter 过滤离线/隔离节点
type NodeSchedulableFilter struct{}

func (NodeSchedulableFilter) Name() string { return "NodeSchedulable" }
func (NodeSchedulableFilter) Filter(task *Task, node *NodeInfo) *Status {
	if !node.Schedulable {
		return &Status{"NodeSchedulable", "node is offline or cordoned"}
	}
	return nil
}

// NodeAffinityFilter 节点选择器与 ConstrainNodeIDs 约束
type NodeAffinityFilter struct{}

func (NodeAffinityFilter) Name() string { return "NodeAffinity" }
func (NodeAffinityFilter) Filter(task *Task, node *NodeInfo) *Status {
	if req := task.ResourceReq; req != nil && len(req.ConstrainNodeIDs) > 0 {
		found := false
		for _, n := range req.ConstrainNodeIDs {
			if n == node.Name {
				found = true
				break
			}
		}
		if !found {
			return &Status{"NodeAffinity", "node not in constrain_node_ids"}
		}
	}
	for k, v := range task.NodeSelector {
		if node.Labels[k] != v {
			return &Status{"NodeAffinity", fmt.Sprintf("node selector %s=%s not matched", k, v)}
		}
	}
	return nil
}

// TenantIsolationFilter 租户独占节点隔离：带 tenant.dedicated 标签的节点只运行该租户任务
type TenantIsolationFilter struct{}

func (TenantIsolationFilter) Name() string { return "TenantIsolation" }
func (TenantIsolationFilter) Filter(task *Task, node *NodeInfo) *Status {
	if owner := node.Labels[LabelTenantDedicated]; owner != "" && owner != task.TenantID {
		return &Status{"TenantIsolation", "node dedicated to tenant " + owner}
	}
	return nil
}

// ---------------- Score plugins ----------------

func nodeMemory(node *NodeInfo, rtype pool.ResourceType) (used, total int64) {
	for _, d := range node.Devices {
		if d.Type == rtype {
			used += d.UsedMemory
			total += d.TotalMemory
		}
	}
	return
}

func placementMemory(p *Placement) int64 {
	var m int64
	for _, s := range p.Slices {
		m += s.MemoryGB
	}
	return m
}

// BinPackScore MostAllocated：优先填满节点，为大任务保留整机
type BinPackScore struct{}

func (BinPackScore) Name() string { return "BinPack" }
func (BinPackScore) Score(_ *CycleState, task *Task, node *NodeInfo, p *Placement) int64 {
	used, total := nodeMemory(node, task.ResourceReq.ResourceType)
	if total == 0 {
		return 0
	}
	return (used + placementMemory(p)) * 100 / total
}

// LoadBalanceScore LeastAllocated：负载均衡，分散到空闲节点
type LoadBalanceScore struct{}

func (LoadBalanceScore) Name() string { return "LoadBalance" }
func (LoadBalanceScore) Score(_ *CycleState, task *Task, node *NodeInfo, p *Placement) int64 {
	used, total := nodeMemory(node, task.ResourceReq.ResourceType)
	if total == 0 {
		return 0
	}
	return 100 - (used+placementMemory(p))*100/total
}

// TopologyScore NVLink / NUMA / PCIe 拓扑亲和，以及 Gang 成员 RDMA 域亲和
type TopologyScore struct{}

func (TopologyScore) Name() string { return "Topology" }
func (TopologyScore) Score(state *CycleState, task *Task, node *NodeInfo, p *Placement) int64 {
	score := p.TopologyScore
	if task.GangGroup != "" && state != nil && len(state.GangDomains) > 0 {
		domain := node.Labels[LabelRDMADomain]
		if domain != "" && state.GangDomains[domain] > 0 {
			return (score + 100) / 2
		}
		return score / 2
	}
	return score
}

// FragmentationScore 切分类请求优先落在已部分占用的卡上，避免打碎空闲整卡
type FragmentationScore struct{}

func (FragmentationScore) Name() string { return "Fragmentation" }
func (FragmentationScore) Score(_ *CycleState, task *Task, node *NodeInfo, p *Placement) int64 {
	if p.Mode == pool.ShareModeExclusive || len(p.Slices) == 0 {
		return 100
	}
	idle := 0
	for _, s := range p.Slices {
		for _, d := range node.Devices {
			if d.ID == s.ResourceID && d.Holders == 0 {
				idle++
			}
		}
	}
	return int64(100 - idle*100/len(p.Slices))
}

// DeviceHealthScore 降低亚健康（降级）设备的优先级
type DeviceHealthScore struct{}

func (DeviceHealthScore) Name() string { return "DeviceHealth" }
func (DeviceHealthScore) Score(_ *CycleState, task *Task, node *NodeInfo, p *Placement) int64 {
	for _, s := range p.Slices {
		for _, d := range node.Devices {
			if d.ID == s.ResourceID && d.Health == pool.DeviceHealthDegraded {
				return 30
			}
		}
	}
	return 100
}

// ---------------- 设备落位（拓扑感知） ----------------

func topoKey(d *pool.Resource, level string) string {
	t := d.Topology
	switch level {
	case "nvlink":
		if t != nil && t.NVLinkGroup != "" {
			return t.NVLinkGroup
		}
		return ""
	case "numa":
		if t != nil {
			return fmt.Sprintf("numa-%d", t.NUMANode)
		}
		return ""
	}
	return ""
}

// PlaceOnNode 在节点内挑选设备：多卡优先同 NVLink 组，其次同 NUMA；单卡切分优先已占用设备。
func PlaceOnNode(task *Task, node *NodeInfo, count int, overcommit float64) (*Placement, error) {
	req := *task.ResourceReq
	req.Count = count
	type cand struct {
		dev   *pool.Resource
		slice pool.AllocationSlice
	}
	var cands []cand
	var lastErr error
	for _, d := range node.Devices {
		if d.Type != req.ResourceType {
			continue
		}
		s, err := pool.DeviceFit(d, &req, overcommit)
		if err != nil {
			lastErr = err
			continue
		}
		cands = append(cands, cand{d, s})
	}
	if len(cands) < count {
		if lastErr == nil {
			lastErr = fmt.Errorf("no %s device", req.ResourceType)
		}
		return nil, fmt.Errorf("need %d devices, %d fit (%v)", count, len(cands), lastErr)
	}
	mode := pool.ResolveShareMode(&req, cands[0].dev.TotalCores)

	// 空闲设备数按 NVLink 组统计，整卡请求优先使用"已被打散"组内的卡，保留完整 NVLink 组给大任务
	groupFree := make(map[string]int)
	for _, d := range node.Devices {
		if d.Holders == 0 {
			groupFree[topoKey(d, "nvlink")]++
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i].dev, cands[j].dev
		if mode != pool.ShareModeExclusive && (a.Holders > 0) != (b.Holders > 0) {
			return a.Holders > 0
		}
		if mode != pool.ShareModeExclusive {
			if fa, fb := a.TotalMemory-a.UsedMemory, b.TotalMemory-b.UsedMemory; fa != fb {
				return fa < fb
			}
		}
		if ga, gb := groupFree[topoKey(a, "nvlink")], groupFree[topoKey(b, "nvlink")]; ga != gb {
			return ga < gb
		}
		return a.ID < b.ID
	})

	build := func(cs []cand, topo int64) *Placement {
		p := &Placement{Node: node.Name, Mode: mode, TopologyScore: topo}
		for _, c := range cs[:count] {
			p.Slices = append(p.Slices, c.slice)
		}
		return p
	}
	if count == 1 {
		return build(cands, 100), nil
	}
	for _, level := range []struct {
		key   string
		score int64
	}{{"nvlink", 100}, {"numa", 70}} {
		groups := make(map[string][]cand)
		var keys []string
		for _, c := range cands {
			k := topoKey(c.dev, level.key)
			if k == "" {
				continue
			}
			if _, ok := groups[k]; !ok {
				keys = append(keys, k)
			}
			groups[k] = append(groups[k], c)
		}
		// best-fit：选择恰好能放下的最小组，减少对其他组的破坏
		sort.Slice(keys, func(i, j int) bool {
			if len(groups[keys[i]]) != len(groups[keys[j]]) {
				return len(groups[keys[i]]) < len(groups[keys[j]])
			}
			return keys[i] < keys[j]
		})
		for _, k := range keys {
			if len(groups[k]) >= count {
				return build(groups[k], level.score), nil
			}
		}
	}
	return build(cands, 30), nil
}

// ---------------- 调度器配置 ----------------

// PluginConfig 插件开关与权重
type PluginConfig struct {
	Enabled bool `json:"enabled"`
	Weight  int  `json:"weight"`
}

// Config 调度器配置（可通过 API 热更新）
type Config struct {
	Filters          map[string]bool         `json:"filters"`
	Scores           map[string]PluginConfig `json:"scores"`
	EnablePreemption bool                    `json:"enable_preemption"`
	EnableGang       bool                    `json:"enable_gang"`
	EnableElastic    bool                    `json:"enable_elastic"`
	EnableBackfill   bool                    `json:"enable_backfill"`
	// StarvationSeconds 高优任务等待超过该时长后，阻止低优任务回填同类资源
	StarvationSeconds int     `json:"starvation_seconds"`
	CoreOvercommit    float64 `json:"core_overcommit"`
	MaxRetries        int     `json:"max_retries"`
	IntervalMillis    int     `json:"interval_millis"`
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		Filters: map[string]bool{"NodeSchedulable": true, "NodeAffinity": true, "TenantIsolation": true},
		Scores: map[string]PluginConfig{
			"BinPack":       {Enabled: true, Weight: 2},
			"LoadBalance":   {Enabled: false, Weight: 1},
			"Topology":      {Enabled: true, Weight: 3},
			"Fragmentation": {Enabled: true, Weight: 2},
			"DeviceHealth":  {Enabled: true, Weight: 1},
		},
		EnablePreemption:  true,
		EnableGang:        true,
		EnableElastic:     true,
		EnableBackfill:    true,
		StarvationSeconds: 300,
		CoreOvercommit:    2.0,
		MaxRetries:        3,
		IntervalMillis:    1000,
	}
}

var (
	allFilters = []FilterPlugin{NodeSchedulableFilter{}, NodeAffinityFilter{}, TenantIsolationFilter{}}
	allScores  = []ScorePlugin{BinPackScore{}, LoadBalanceScore{}, TopologyScore{}, FragmentationScore{}, DeviceHealthScore{}}
)

// scoreWeights 合并全局配置与任务类型策略的权重调整
func scoreWeights(cfg Config, strategy SchedulingStrategy) map[string]int {
	w := make(map[string]int)
	for name, pc := range cfg.Scores {
		if pc.Enabled {
			w[name] = pc.Weight
		}
	}
	if tuner, ok := strategy.(interface{ AdjustWeights(map[string]int) }); ok {
		tuner.AdjustWeights(w)
	}
	return w
}

// NodeEvaluation 单节点评估结果（供 K8s 调度扩展器与调试接口使用）
type NodeEvaluation struct {
	Node        string           `json:"node"`
	Feasible    bool             `json:"feasible"`
	Reason      string           `json:"reason,omitempty"`
	Score       int64            `json:"score"`
	ScoreDetail map[string]int64 `json:"score_detail,omitempty"`
	Devices     []string         `json:"devices,omitempty"`
}

// evaluate 对快照中所有节点执行 Filter + 落位 + Score
func evaluate(cfg Config, weights map[string]int, snap *Snapshot, state *CycleState, task *Task, count int, only map[string]bool) ([]*Placement, []NodeEvaluation) {
	var feasible []*Placement
	var evals []NodeEvaluation
	for _, name := range snap.order {
		if only != nil && !only[name] {
			continue
		}
		node := snap.Nodes[name]
		ev := NodeEvaluation{Node: name}
		var failed *Status
		for _, f := range allFilters {
			if !cfg.Filters[f.Name()] {
				continue
			}
			if st := f.Filter(task, node); st != nil {
				failed = st
				break
			}
		}
		if failed != nil {
			ev.Reason = failed.Error()
			evals = append(evals, ev)
			continue
		}
		p, err := PlaceOnNode(task, node, count, snap.CoreOvercommit)
		if err != nil {
			ev.Reason = "DeviceFit: " + err.Error()
			evals = append(evals, ev)
			continue
		}
		p.ScoreDetail = make(map[string]int64)
		var sum, wsum int64
		for _, sp := range allScores {
			wt, ok := weights[sp.Name()]
			if !ok || wt <= 0 {
				continue
			}
			v := sp.Score(state, task, node, p)
			p.ScoreDetail[sp.Name()] = v
			sum += v * int64(wt)
			wsum += int64(wt)
		}
		if wsum > 0 {
			p.Score = sum / wsum
		}
		ev.Feasible, ev.Score, ev.ScoreDetail, ev.Devices = true, p.Score, p.ScoreDetail, p.ResourceIDs()
		evals = append(evals, ev)
		feasible = append(feasible, p)
	}
	sort.SliceStable(feasible, func(i, j int) bool { return feasible[i].Score > feasible[j].Score })
	return feasible, evals
}

// summarizeFailures 汇总不可调度原因，类似 kube-scheduler 的 "0/3 nodes are available: ..."
func summarizeFailures(evals []NodeEvaluation) string {
	if len(evals) == 0 {
		return "no nodes with matching resource type"
	}
	counts := make(map[string]int)
	var keys []string
	for _, e := range evals {
		if e.Feasible {
			continue
		}
		reason := e.Reason
		if i := strings.Index(reason, " ("); i > 0 {
			reason = reason[:i]
		}
		if _, ok := counts[reason]; !ok {
			keys = append(keys, reason)
		}
		counts[reason]++
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d %s", counts[k], k))
	}
	return fmt.Sprintf("0/%d nodes are available: %s", len(evals), strings.Join(parts, "; "))
}
