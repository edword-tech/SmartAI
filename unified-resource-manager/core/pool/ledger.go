/*
 * Unified Resource Manager - Device Ledger
 *
 * 设备账本：GPU/NPU 适配器共用的设备容量与分配记账实现。
 * - 整卡独占 (exclusive)、显存+算力切分 (shared, 即 vGPU/vNPU)、MIG 固定切片、分时复用 (timeslice, 允许算力超卖)
 * - 分配/释放幂等记账，支持按调度器给定的设备 ID 精确落位 (pinned placement)
 * - 设备拓扑 (NUMA / PCIe Switch / NVLink 组 / RDMA 网卡)、隔离 (cordon)、健康状态
 */

package pool

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ShareMode 设备共享方式
type ShareMode string

const (
	ShareModeExclusive ShareMode = "exclusive" // 整卡独占
	ShareModeShared    ShareMode = "shared"    // 显存+算力硬切分（vGPU / vNPU）
	ShareModeMIG       ShareMode = "mig"       // NVIDIA MIG 固定规格切片
	ShareModeTimeSlice ShareMode = "timeslice" // 分时复用，算力可按超卖比例超分
)

// DeviceHealth 设备健康状态
const (
	DeviceHealthHealthy  = "healthy"
	DeviceHealthDegraded = "degraded"
	DeviceHealthFailed   = "failed"
)

// DeviceTopology 设备拓扑信息，用于拓扑感知调度
type DeviceTopology struct {
	NUMANode    int    `json:"numa_node"`
	PCIeSwitch  string `json:"pcie_switch,omitempty"`
	NVLinkGroup string `json:"nvlink_group,omitempty"` // 同组设备之间 NVLink/HCCS 全互联
	RDMANic     string `json:"rdma_nic,omitempty"`     // 亲和的 RDMA 网卡
}

// DeviceSpec 设备规格（由设备发现或节点注册提供）
type DeviceSpec struct {
	DeviceID    string            `json:"device_id"`
	Model       string            `json:"model"`
	TotalMemory int64             `json:"total_memory"` // GB
	TotalCores  int               `json:"total_cores"`
	Topology    *DeviceTopology   `json:"topology,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// MIGProfile MIG 切片规格
type MIGProfile struct {
	Name          string `json:"name"`
	ComputeSlices int    `json:"compute_slices"` // 每卡共 7 份
	MemoryGB      int64  `json:"memory_gb"`
}

// MIGComputeSlices 单卡 MIG 计算切片总数
const MIGComputeSlices = 7

var migProfilesByMemory = map[int64][]MIGProfile{
	40: {{"1g.5gb", 1, 5}, {"2g.10gb", 2, 10}, {"3g.20gb", 3, 20}, {"4g.20gb", 4, 20}, {"7g.40gb", 7, 40}},
	80: {{"1g.10gb", 1, 10}, {"2g.20gb", 2, 20}, {"3g.40gb", 3, 40}, {"4g.40gb", 4, 40}, {"7g.80gb", 7, 80}},
}

// MIGProfilesFor 返回设备支持的 MIG 规格；不支持 MIG 的设备返回 nil
func MIGProfilesFor(model string, totalMemory int64) []MIGProfile {
	m := strings.ToUpper(model)
	if !(strings.Contains(m, "A100") || strings.Contains(m, "A800") || strings.Contains(m, "H100") || strings.Contains(m, "H800") || strings.Contains(m, "H20")) {
		return nil
	}
	return migProfilesByMemory[totalMemory]
}

func findMIGProfile(model string, totalMemory int64, name string) (MIGProfile, bool) {
	for _, p := range MIGProfilesFor(model, totalMemory) {
		if p.Name == name {
			return p, true
		}
	}
	return MIGProfile{}, false
}

// AllocationSlice 一次分配在单个设备上的切片
type AllocationSlice struct {
	ResourceID string `json:"resource_id"`
	NodeID     string `json:"node_id"`
	DeviceID   string `json:"device_id"`
	MemoryGB   int64  `json:"memory_gb"`
	Cores      int    `json:"cores"`
	MIGProfile string `json:"mig_profile,omitempty"`
	MIGSlices  int    `json:"mig_slices,omitempty"`
}

// LedgerAllocation 账本中的一次分配
type LedgerAllocation struct {
	ID        string            `json:"id"`
	TaskID    string            `json:"task_id"`
	TenantID  string            `json:"tenant_id,omitempty"`
	Mode      ShareMode         `json:"mode"`
	Slices    []AllocationSlice `json:"slices"`
	CreatedAt time.Time         `json:"created_at"`
}

type ledgerDevice struct {
	spec      DeviceSpec
	nodeID    string
	usedMem   int64
	usedCores int
	migSlices int
	mode      ShareMode
	holders   map[string]struct{}
	cordoned  bool
	health    string
	updatedAt time.Time
}

// Ledger 设备账本
type Ledger struct {
	rtype          ResourceType
	devices        map[string]*ledgerDevice
	allocations    map[string]*LedgerAllocation
	coreOvercommit float64
	mu             sync.RWMutex
}

// NewLedger 创建设备账本
func NewLedger(rtype ResourceType) *Ledger {
	return &Ledger{
		rtype:          rtype,
		devices:        make(map[string]*ledgerDevice),
		allocations:    make(map[string]*LedgerAllocation),
		coreOvercommit: 2.0,
	}
}

// MakeResourceID 由节点与设备 ID 生成资源 ID
func MakeResourceID(nodeID, deviceID string) string {
	return nodeID + "-" + deviceID
}

// SetCoreOvercommit 设置分时复用模式下的算力超卖比例（>=1）
func (l *Ledger) SetCoreOvercommit(ratio float64) {
	if ratio < 1 {
		ratio = 1
	}
	l.mu.Lock()
	l.coreOvercommit = ratio
	l.mu.Unlock()
}

// CoreOvercommit 返回当前算力超卖比例
func (l *Ledger) CoreOvercommit() float64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.coreOvercommit
}

// SyncNode 以给定设备列表覆盖节点设备；仍有分配的设备保留其占用
func (l *Ledger) SyncNode(nodeID string, specs []DeviceSpec) {
	l.mu.Lock()
	defer l.mu.Unlock()
	keep := make(map[string]bool, len(specs))
	for _, spec := range specs {
		id := MakeResourceID(nodeID, spec.DeviceID)
		keep[id] = true
		if dev, ok := l.devices[id]; ok {
			dev.spec = spec
			dev.updatedAt = time.Now()
			continue
		}
		l.devices[id] = &ledgerDevice{
			spec:      spec,
			nodeID:    nodeID,
			holders:   make(map[string]struct{}),
			health:    DeviceHealthHealthy,
			updatedAt: time.Now(),
		}
	}
	for id, dev := range l.devices {
		if dev.nodeID == nodeID && !keep[id] && len(dev.holders) == 0 {
			delete(l.devices, id)
		}
	}
}

// NodeIDs 返回账本中的所有节点
func (l *Ledger) NodeIDs() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	set := make(map[string]bool)
	for _, d := range l.devices {
		set[d.nodeID] = true
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// HasResource 判断资源是否属于该账本
func (l *Ledger) HasResource(resourceID string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	_, ok := l.devices[resourceID]
	return ok
}

func (l *Ledger) stateOf(d *ledgerDevice) ResourceState {
	switch {
	case d.health == DeviceHealthFailed:
		return StateError
	case d.cordoned:
		return StateMaintenance
	case d.mode == ShareModeExclusive:
		return StateAllocated
	case d.mode == ShareModeMIG && d.migSlices >= MIGComputeSlices:
		return StateAllocated
	case d.mode == ShareModeShared && (d.usedMem >= d.spec.TotalMemory || d.usedCores >= d.spec.TotalCores):
		return StateAllocated
	case d.mode == ShareModeTimeSlice && (d.usedMem >= d.spec.TotalMemory || float64(d.usedCores) >= float64(d.spec.TotalCores)*l.coreOvercommit):
		return StateAllocated
	}
	return StateAvailable
}

func (l *Ledger) toResource(id string, d *ledgerDevice) *Resource {
	labels := make(map[string]string, len(d.spec.Labels)+4)
	for k, v := range d.spec.Labels {
		labels[k] = v
	}
	labels["model"] = d.spec.Model
	if d.mode != "" {
		labels["share_mode"] = string(d.mode)
	}
	if d.spec.Topology != nil {
		labels["topology.numa"] = fmt.Sprintf("%d", d.spec.Topology.NUMANode)
		if d.spec.Topology.NVLinkGroup != "" {
			labels["topology.nvlink_group"] = d.spec.Topology.NVLinkGroup
		}
		if d.spec.Topology.PCIeSwitch != "" {
			labels["topology.pcie_switch"] = d.spec.Topology.PCIeSwitch
		}
		if d.spec.Topology.RDMANic != "" {
			labels["topology.rdma_nic"] = d.spec.Topology.RDMANic
		}
	}
	var topo *DeviceTopology
	if d.spec.Topology != nil {
		t := *d.spec.Topology
		topo = &t
	}
	return &Resource{
		ID:          id,
		Type:        l.rtype,
		State:       l.stateOf(d),
		NodeID:      d.nodeID,
		DeviceID:    d.spec.DeviceID,
		Model:       d.spec.Model,
		TotalMemory: d.spec.TotalMemory,
		UsedMemory:  d.usedMem,
		TotalCores:  d.spec.TotalCores,
		UsedCores:   d.usedCores,
		MIGSlices:   d.migSlices,
		ShareMode:   d.mode,
		Holders:     len(d.holders),
		Health:      d.health,
		Cordoned:    d.cordoned,
		Topology:    topo,
		Labels:      labels,
		UpdatedAt:   d.updatedAt,
	}
}

// List 列出账本资源快照（按资源 ID 排序），nodeFilter 为空表示全部
func (l *Ledger) List(nodeFilter func(string) bool) []*Resource {
	l.mu.RLock()
	defer l.mu.RUnlock()
	ids := make([]string, 0, len(l.devices))
	for id, d := range l.devices {
		if nodeFilter == nil || nodeFilter(d.nodeID) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	out := make([]*Resource, 0, len(ids))
	for _, id := range ids {
		out = append(out, l.toResource(id, l.devices[id]))
	}
	return out
}

// Get 获取单个资源快照
func (l *Ledger) Get(resourceID string) (*Resource, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	d, ok := l.devices[resourceID]
	if !ok {
		return nil, false
	}
	return l.toResource(resourceID, d), true
}

// ResolveShareMode 根据请求与设备推断共享方式
func ResolveShareMode(req *AllocationRequest, totalCores int) ShareMode {
	if req.ShareMode != "" {
		return req.ShareMode
	}
	if req.MIGProfile != "" {
		return ShareModeMIG
	}
	if (req.Cores == 0 && req.MemoryGB == 0) || (totalCores > 0 && req.Cores >= totalCores) {
		return ShareModeExclusive
	}
	return ShareModeShared
}

// DeviceFit 计算单设备能否满足请求，返回在该设备上的切片
func DeviceFit(r *Resource, req *AllocationRequest, coreOvercommit float64) (AllocationSlice, error) {
	slice := AllocationSlice{ResourceID: r.ID, NodeID: r.NodeID, DeviceID: r.DeviceID}
	if r.Health == DeviceHealthFailed {
		return slice, fmt.Errorf("device %s unhealthy", r.ID)
	}
	if r.Cordoned {
		return slice, fmt.Errorf("device %s cordoned", r.ID)
	}
	if ReservedForDebug(r) && !WantsDebug(req) {
		return slice, fmt.Errorf("device %s reserved for debug/probe", r.ID)
	}
	mode := ResolveShareMode(req, r.TotalCores)
	if r.ShareMode != "" && r.ShareMode != mode {
		return slice, fmt.Errorf("device %s in %s mode", r.ID, r.ShareMode)
	}
	freeMem := r.TotalMemory - r.UsedMemory
	switch mode {
	case ShareModeExclusive:
		if r.Holders > 0 {
			return slice, fmt.Errorf("device %s busy", r.ID)
		}
		if req.MemoryGB > r.TotalMemory {
			return slice, fmt.Errorf("device %s memory %dGB < %dGB", r.ID, r.TotalMemory, req.MemoryGB)
		}
		slice.MemoryGB, slice.Cores = r.TotalMemory, r.TotalCores
	case ShareModeMIG:
		p, ok := findMIGProfile(r.Model, r.TotalMemory, req.MIGProfile)
		if !ok {
			return slice, fmt.Errorf("device %s does not support MIG profile %q", r.ID, req.MIGProfile)
		}
		if r.MIGSlices+p.ComputeSlices > MIGComputeSlices || p.MemoryGB > freeMem {
			return slice, fmt.Errorf("device %s has no free MIG slice for %s", r.ID, p.Name)
		}
		slice.MemoryGB = p.MemoryGB
		slice.Cores = p.ComputeSlices * r.TotalCores / MIGComputeSlices
		slice.MIGProfile = p.Name
		slice.MIGSlices = p.ComputeSlices
	case ShareModeShared, ShareModeTimeSlice:
		mem, cores := req.MemoryGB, req.Cores
		if mem <= 0 && r.TotalCores > 0 {
			mem = r.TotalMemory * int64(cores) / int64(r.TotalCores)
		}
		if cores <= 0 && r.TotalMemory > 0 {
			cores = int(int64(r.TotalCores) * mem / r.TotalMemory)
		}
		limit := float64(r.TotalCores)
		if mode == ShareModeTimeSlice {
			limit *= coreOvercommit
		}
		if mem > freeMem {
			return slice, fmt.Errorf("device %s free memory %dGB < %dGB", r.ID, freeMem, mem)
		}
		if float64(r.UsedCores+cores) > limit {
			return slice, fmt.Errorf("device %s free cores insufficient", r.ID)
		}
		slice.MemoryGB, slice.Cores = mem, cores
	default:
		return slice, fmt.Errorf("unknown share mode %q", mode)
	}
	return slice, nil
}

// Allocate 分配资源。若 req.ResourceIDs 非空则按指定设备精确落位，否则按 binpack 首次适配。
func (l *Ledger) Allocate(req *AllocationRequest, nodeFilter func(string) bool) (*LedgerAllocation, []*Resource, error) {
	if req == nil || req.Count <= 0 {
		return nil, nil, ErrInvalidRequest
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	var slices []AllocationSlice
	if len(req.ResourceIDs) > 0 {
		if len(req.ResourceIDs) != req.Count {
			return nil, nil, fmt.Errorf("pinned resource count %d != requested %d", len(req.ResourceIDs), req.Count)
		}
		seen := make(map[string]bool)
		for _, id := range req.ResourceIDs {
			d, ok := l.devices[id]
			if !ok {
				return nil, nil, fmt.Errorf("resource not found: %s", id)
			}
			if seen[id] {
				return nil, nil, fmt.Errorf("duplicate resource %s", id)
			}
			seen[id] = true
			s, err := DeviceFit(l.toResource(id, d), req, l.coreOvercommit)
			if err != nil {
				return nil, nil, err
			}
			slices = append(slices, s)
		}
	} else {
		slices = l.selectBinpack(req, nodeFilter)
		if len(slices) < req.Count {
			return nil, nil, ErrInsufficientResources
		}
	}

	alloc := &LedgerAllocation{
		ID:        uuid.New().String(),
		TaskID:    req.TaskID,
		TenantID:  req.TenantID,
		Slices:    slices,
		CreatedAt: time.Now(),
	}
	var out []*Resource
	for _, s := range slices {
		d := l.devices[s.ResourceID]
		mode := ResolveShareMode(req, d.spec.TotalCores)
		alloc.Mode = mode
		d.mode = mode
		d.usedMem += s.MemoryGB
		d.usedCores += s.Cores
		d.migSlices += s.MIGSlices
		d.holders[alloc.ID] = struct{}{}
		d.updatedAt = time.Now()
		out = append(out, l.toResource(s.ResourceID, d))
	}
	l.allocations[alloc.ID] = alloc
	return alloc, out, nil
}

// selectBinpack 首选单节点放下全部设备，优先已部分占用的设备与更满的节点，减少碎片
func (l *Ledger) selectBinpack(req *AllocationRequest, nodeFilter func(string) bool) []AllocationSlice {
	constrain := make(map[string]bool, len(req.ConstrainNodeIDs))
	for _, n := range req.ConstrainNodeIDs {
		constrain[n] = true
	}
	byNode := make(map[string][]AllocationSlice)
	partial := make(map[string]bool)
	debugPref := make(map[string]bool)
	nodeUsed := make(map[string]int64)
	wantDebug := WantsDebug(req)
	for id, d := range l.devices {
		if nodeFilter != nil && !nodeFilter(d.nodeID) {
			continue
		}
		if len(constrain) > 0 && !constrain[d.nodeID] {
			continue
		}
		nodeUsed[d.nodeID] += d.usedMem
		res := l.toResource(id, d)
		s, err := DeviceFit(res, req, l.coreOvercommit)
		if err != nil {
			continue
		}
		byNode[d.nodeID] = append(byNode[d.nodeID], s)
		partial[id] = len(d.holders) > 0
		if wantDebug && ReservedForDebug(res) {
			debugPref[id] = true
		}
	}
	nodes := make([]string, 0, len(byNode))
	for n, s := range byNode {
		sort.Slice(s, func(i, j int) bool {
			if debugPref[s[i].ResourceID] != debugPref[s[j].ResourceID] {
				return debugPref[s[i].ResourceID]
			}
			if partial[s[i].ResourceID] != partial[s[j].ResourceID] {
				return partial[s[i].ResourceID]
			}
			return s[i].ResourceID < s[j].ResourceID
		})
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodeUsed[nodes[i]] != nodeUsed[nodes[j]] {
			return nodeUsed[nodes[i]] > nodeUsed[nodes[j]]
		}
		return nodes[i] < nodes[j]
	})
	for _, n := range nodes {
		if len(byNode[n]) >= req.Count {
			return byNode[n][:req.Count]
		}
	}
	var out []AllocationSlice
	for _, n := range nodes {
		for _, s := range byNode[n] {
			if len(out) == req.Count {
				return out
			}
			out = append(out, s)
		}
	}
	return out
}

// Release 释放分配
func (l *Ledger) Release(allocationID string) (*LedgerAllocation, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	alloc, ok := l.allocations[allocationID]
	if !ok {
		return nil, ErrAllocationNotFound
	}
	for _, s := range alloc.Slices {
		d, ok := l.devices[s.ResourceID]
		if !ok {
			continue
		}
		d.usedMem -= s.MemoryGB
		d.usedCores -= s.Cores
		d.migSlices -= s.MIGSlices
		delete(d.holders, allocationID)
		if len(d.holders) == 0 {
			d.usedMem, d.usedCores, d.migSlices, d.mode = 0, 0, 0, ""
		}
		d.updatedAt = time.Now()
	}
	delete(l.allocations, allocationID)
	return alloc, nil
}

// GetAllocation 查询分配
func (l *Ledger) GetAllocation(allocationID string) (*LedgerAllocation, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	a, ok := l.allocations[allocationID]
	return a, ok
}

// AllocationsOnResource 返回某设备上的分配 ID
func (l *Ledger) AllocationsOnResource(resourceID string) []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	d, ok := l.devices[resourceID]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(d.holders))
	for id := range d.holders {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// LabelPurpose 设备用途标签
const LabelPurpose = "purpose"
const PurposeDebug = "debug"

// ReservedForDebug 设备是否为调测预留卡（业务任务不可占用）
func ReservedForDebug(r *Resource) bool {
	return r != nil && r.Labels[LabelPurpose] == PurposeDebug
}

// WantsDebug 分配请求是否允许使用调测预留卡
func WantsDebug(req *AllocationRequest) bool {
	return req != nil && req.Labels[LabelPurpose] == PurposeDebug
}

// SetDeviceLabel 设置设备标签（调测预留等）
func (l *Ledger) SetDeviceLabel(resourceID, key, value string) error {
	if key == "" {
		return fmt.Errorf("label key required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	d, ok := l.devices[resourceID]
	if !ok {
		return fmt.Errorf("resource not found: %s", resourceID)
	}
	if d.spec.Labels == nil {
		d.spec.Labels = map[string]string{}
	}
	if value == "" {
		delete(d.spec.Labels, key)
	} else {
		d.spec.Labels[key] = value
	}
	d.updatedAt = time.Now()
	return nil
}

// SetCordon 隔离/解除隔离设备（隔离后不再接收新分配）
func (l *Ledger) SetCordon(resourceID string, cordoned bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	d, ok := l.devices[resourceID]
	if !ok {
		return fmt.Errorf("resource not found: %s", resourceID)
	}
	d.cordoned = cordoned
	d.updatedAt = time.Now()
	return nil
}

// SetHealth 设置设备健康状态
func (l *Ledger) SetHealth(resourceID, health string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	d, ok := l.devices[resourceID]
	if !ok {
		return fmt.Errorf("resource not found: %s", resourceID)
	}
	d.health = health
	d.updatedAt = time.Now()
	return nil
}

// SetState 兼容 ResourcePool.UpdateResourceState 语义
func (l *Ledger) SetState(resourceID string, state ResourceState) error {
	switch state {
	case StateMaintenance:
		return l.SetCordon(resourceID, true)
	case StateError:
		return l.SetHealth(resourceID, DeviceHealthFailed)
	case StateAvailable:
		if err := l.SetCordon(resourceID, false); err != nil {
			return err
		}
		return l.SetHealth(resourceID, DeviceHealthHealthy)
	default:
		return fmt.Errorf("state %s cannot be set directly", state)
	}
}

// DeviceHooks 分配/释放时对接底层虚拟化服务的钩子
type DeviceHooks interface {
	OnAllocate(ctx context.Context, alloc *LedgerAllocation) error
	OnRelease(ctx context.Context, alloc *LedgerAllocation) error
}

// MetricsProvider 提供设备实时指标（如来自 DCGM / xpu-exporter）
type MetricsProvider func(resourceID string) (*ResourceMetrics, bool)

// LedgerPool 基于账本的 ResourcePool 实现；nodes 为空表示纳管账本内全部节点
type LedgerPool struct {
	Ledger  *Ledger
	Name    string
	rtype   ResourceType
	nodes   map[string]bool
	hooks   DeviceHooks
	metrics MetricsProvider
	created time.Time
	mu      sync.RWMutex
}

// NewLedgerPool 创建账本资源池
func NewLedgerPool(ledger *Ledger, name string, rtype ResourceType, nodes []string, hooks DeviceHooks) *LedgerPool {
	p := &LedgerPool{Ledger: ledger, Name: name, rtype: rtype, nodes: make(map[string]bool), hooks: hooks, created: time.Now()}
	for _, n := range nodes {
		p.nodes[n] = true
	}
	return p
}

// SetMetricsProvider 设置实时指标来源
func (p *LedgerPool) SetMetricsProvider(m MetricsProvider) {
	p.mu.Lock()
	p.metrics = m
	p.mu.Unlock()
}

// AddNodes 将节点纳入资源池
func (p *LedgerPool) AddNodes(nodes ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, n := range nodes {
		p.nodes[n] = true
	}
}

func (p *LedgerPool) nodeFilter() func(string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.nodes) == 0 {
		return nil
	}
	set := make(map[string]bool, len(p.nodes))
	for n := range p.nodes {
		set[n] = true
	}
	return func(n string) bool { return set[n] }
}

func (p *LedgerPool) owns(resourceID string) bool {
	r, ok := p.Ledger.Get(resourceID)
	if !ok {
		return false
	}
	f := p.nodeFilter()
	return f == nil || f(r.NodeID)
}

// GetPoolInfo 获取资源池信息
func (p *LedgerPool) GetPoolInfo(ctx context.Context) (*PoolInfo, error) {
	info := &PoolInfo{Name: p.Name, Type: p.rtype, CreatedAt: p.created, UpdatedAt: time.Now()}
	nodes := make(map[string]bool)
	for _, r := range p.Ledger.List(p.nodeFilter()) {
		info.TotalCount++
		info.TotalMemory += r.TotalMemory
		info.UsedMemory += r.UsedMemory
		if r.State == StateAvailable {
			info.AvailableCount++
		}
		if r.Holders > 0 {
			info.AllocatedCount++
		}
		nodes[r.NodeID] = true
	}
	for n := range nodes {
		info.Nodes = append(info.Nodes, n)
	}
	sort.Strings(info.Nodes)
	return info, nil
}

// ListResources 列出资源
func (p *LedgerPool) ListResources(ctx context.Context, filters map[string]string) ([]*Resource, error) {
	var out []*Resource
	for _, r := range p.Ledger.List(p.nodeFilter()) {
		if MatchResourceFilters(r, filters) {
			out = append(out, r)
		}
	}
	return out, nil
}

// GetResource 获取资源
func (p *LedgerPool) GetResource(ctx context.Context, resourceID string) (*Resource, error) {
	if !p.owns(resourceID) {
		return nil, fmt.Errorf("resource not found: %s", resourceID)
	}
	r, _ := p.Ledger.Get(resourceID)
	return r, nil
}

// AllocateResources 分配资源
func (p *LedgerPool) AllocateResources(ctx context.Context, req *AllocationRequest) (*AllocationResult, error) {
	if req == nil {
		return nil, ErrInvalidRequest
	}
	if req.ResourceType != "" && req.ResourceType != p.rtype {
		return nil, fmt.Errorf("invalid resource type %s for %s pool", req.ResourceType, p.rtype)
	}
	for _, id := range req.ResourceIDs {
		if !p.owns(id) {
			return nil, fmt.Errorf("resource %s not in pool %s", id, p.Name)
		}
	}
	alloc, resources, err := p.Ledger.Allocate(req, p.nodeFilter())
	if err != nil {
		return nil, err
	}
	if p.hooks != nil {
		if err := p.hooks.OnAllocate(ctx, alloc); err != nil {
			_, _ = p.Ledger.Release(alloc.ID)
			return nil, fmt.Errorf("bind devices: %w", err)
		}
	}
	result := &AllocationResult{AllocationID: alloc.ID, Resources: resources, Slices: alloc.Slices, ShareMode: alloc.Mode}
	if req.Timeout > 0 {
		result.ExpiresAt = time.Now().Add(req.Timeout)
	}
	return result, nil
}

// ReleaseResources 释放资源
func (p *LedgerPool) ReleaseResources(ctx context.Context, allocationID string) error {
	alloc, ok := p.Ledger.GetAllocation(allocationID)
	if !ok || len(alloc.Slices) == 0 || !p.owns(alloc.Slices[0].ResourceID) {
		return ErrAllocationNotFound
	}
	if p.hooks != nil {
		if err := p.hooks.OnRelease(ctx, alloc); err != nil {
			return fmt.Errorf("unbind devices: %w", err)
		}
	}
	_, err := p.Ledger.Release(allocationID)
	return err
}

// UpdateResourceState 更新资源状态
func (p *LedgerPool) UpdateResourceState(ctx context.Context, resourceID string, state ResourceState) error {
	if !p.owns(resourceID) {
		return fmt.Errorf("resource not found: %s", resourceID)
	}
	return p.Ledger.SetState(resourceID, state)
}

// GetResourceMetrics 获取资源指标；无外部指标源时由账本占用推算
func (p *LedgerPool) GetResourceMetrics(ctx context.Context, resourceID string, duration time.Duration) (*ResourceMetrics, error) {
	r, err := p.GetResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	p.mu.RLock()
	provider := p.metrics
	p.mu.RUnlock()
	if provider != nil {
		if m, ok := provider(resourceID); ok {
			return m, nil
		}
	}
	util := 0.0
	if r.TotalCores > 0 {
		util = float64(r.UsedCores) / float64(r.TotalCores) * 100
		if util > 100 {
			util = 100
		}
	}
	return &ResourceMetrics{
		ResourceID:  resourceID,
		NodeID:      r.NodeID,
		Type:        r.Type,
		Timestamp:   time.Now(),
		MemoryUsed:  r.UsedMemory,
		MemoryTotal: r.TotalMemory,
		CoresUsed:   r.UsedCores,
		CoresTotal:  r.TotalCores,
		Utilization: util,
	}, nil
}

// MatchResourceFilters 通用资源过滤
func MatchResourceFilters(r *Resource, filters map[string]string) bool {
	for k, v := range filters {
		switch k {
		case "node_id":
			if r.NodeID != v {
				return false
			}
		case "state":
			if string(r.State) != v {
				return false
			}
		case "type":
			if string(r.Type) != v {
				return false
			}
		case "health":
			if r.Health != v {
				return false
			}
		default:
			if r.Labels[k] != v {
				return false
			}
		}
	}
	return true
}
