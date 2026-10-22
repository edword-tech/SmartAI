/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2024-2025. All rights reserved.
 * Unified Resource Manager - Resource Pool Management
 */

package pool

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ResourceType 资源类型
type ResourceType string

const (
	ResourceTypeGPU ResourceType = "gpu"
	ResourceTypeNPU ResourceType = "npu"
)

// ResourceState 资源状态
type ResourceState string

const (
	StateAvailable   ResourceState = "available"
	StateAllocated   ResourceState = "allocated"
	StateMaintenance ResourceState = "maintenance"
	StateError       ResourceState = "error"
)

// Resource 表示一个计算资源单元
type Resource struct {
	ID          string       `json:"id"`
	Type        ResourceType `json:"type"`
	State       ResourceState `json:"state"`
	NodeID      string       `json:"node_id"`
	DeviceID    string       `json:"device_id"`
	TotalMemory int64        `json:"total_memory"` // GB
	UsedMemory  int64        `json:"used_memory"`  // GB
	TotalCores  int          `json:"total_cores"`  // 0-100
	UsedCores   int          `json:"used_cores"`   // 0-100
	Model       string          `json:"model,omitempty"`
	MIGSlices   int             `json:"mig_slices,omitempty"` // 已占用 MIG 计算切片数（共 7）
	ShareMode   ShareMode       `json:"share_mode,omitempty"` // 当前共享方式，空表示空闲整卡
	Holders     int             `json:"holders"`              // 当前设备上的分配数
	Health      string          `json:"health,omitempty"`
	Cordoned    bool            `json:"cordoned"`
	Topology    *DeviceTopology `json:"topology,omitempty"`
	Labels      map[string]string `json:"labels"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// ResourcePool 资源池接口
type ResourcePool interface {
	// GetPoolInfo 获取资源池信息
	GetPoolInfo(ctx context.Context) (*PoolInfo, error)
	
	// ListResources 列出所有资源
	ListResources(ctx context.Context, filters map[string]string) ([]*Resource, error)
	
	// GetResource 获取单个资源信息
	GetResource(ctx context.Context, resourceID string) (*Resource, error)
	
	// AllocateResources 分配资源
	AllocateResources(ctx context.Context, req *AllocationRequest) (*AllocationResult, error)
	
	// ReleaseResources 释放资源
	ReleaseResources(ctx context.Context, allocationID string) error
	
	// UpdateResourceState 更新资源状态
	UpdateResourceState(ctx context.Context, resourceID string, state ResourceState) error
	
	// GetResourceMetrics 获取资源指标
	GetResourceMetrics(ctx context.Context, resourceID string, duration time.Duration) (*ResourceMetrics, error)
}

// PoolInfo 资源池信息
type PoolInfo struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Type            ResourceType       `json:"type"`
	TotalCount      int                `json:"total_count"`
	AvailableCount  int                `json:"available_count"`
	AllocatedCount int                `json:"allocated_count"`
	TotalMemory    int64              `json:"total_memory"` // GB
	UsedMemory     int64              `json:"used_memory"`   // GB
	Nodes          []string           `json:"nodes"`
	Labels         map[string]string  `json:"labels,omitempty"` // purpose: training|inference|dev; gpu_model: A100|H100|...
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

// AllocationRequest 资源分配请求
type AllocationRequest struct {
	TaskID       string            `json:"task_id"`
	TaskType     string            `json:"task_type"` // inference, training
	ResourceType ResourceType      `json:"resource_type"`
	Count        int               `json:"count"`
	MemoryGB     int64             `json:"memory_gb"`
	Cores        int               `json:"cores"` // 0-100
	Priority     int               `json:"priority"` // 1-10, 10 is highest
	Labels       map[string]string  `json:"labels"`
	Timeout      time.Duration     `json:"timeout"`
	// ConstrainNodeIDs 仅从指定节点分配（用于大池/集群池）
	ConstrainNodeIDs []string `json:"constrain_node_ids,omitempty"`
	// ConstrainPoolIDs 仅从指定物理池分配
	ConstrainPoolIDs []string `json:"constrain_pool_ids,omitempty"`
	// ShareMode 共享方式：exclusive/shared/mig/timeslice，空则按 cores 推断
	ShareMode ShareMode `json:"share_mode,omitempty"`
	// MIGProfile MIG 规格，如 1g.10gb / 3g.40gb
	MIGProfile string `json:"mig_profile,omitempty"`
	// ResourceIDs 调度器确定的精确落位设备，长度须等于 Count
	ResourceIDs []string `json:"resource_ids,omitempty"`
	TenantID    string   `json:"tenant_id,omitempty"`
}

// AllocationResult 资源分配结果
type AllocationResult struct {
	AllocationID string            `json:"allocation_id"`
	PoolID       string            `json:"pool_id,omitempty"`
	Resources    []*Resource       `json:"resources"`
	Slices       []AllocationSlice `json:"slices,omitempty"`
	ShareMode    ShareMode         `json:"share_mode,omitempty"`
	ExpiresAt    time.Time         `json:"expires_at"`
}

// AllocationRecord 分配记录：用于感知已分配资源及池化剩余
type AllocationRecord struct {
	AllocationID string    `json:"allocation_id"`
	PoolID       string    `json:"pool_id"`
	ResourceIDs  []string  `json:"resource_ids"`
	TaskID       string    `json:"task_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// ResourceMetrics 资源指标
type ResourceMetrics struct {
	ResourceID   string    `json:"resource_id"`
	NodeID       string       `json:"node_id,omitempty"`
	Type         ResourceType `json:"type,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	MemoryUsed   int64     `json:"memory_used"`   // GB
	MemoryTotal  int64     `json:"memory_total"`  // GB
	CoresUsed    int       `json:"cores_used"`
	CoresTotal   int       `json:"cores_total"`
	Utilization  float64   `json:"utilization"`   // 0-100
	Temperature  float64   `json:"temperature"`   // Celsius
	PowerUsage   float64   `json:"power_usage"`   // Watts
}

// ResourceFlavor 资源规格（用于抽象 vGPU / vNPU 等不同切分大小）
type ResourceFlavor struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Type      ResourceType `json:"type"`
	Model     string       `json:"model"`      // 例如 A100/910B3 等
	MemoryGB  int64        `json:"memory_gb"`  // 规格显存大小
	Cores     int          `json:"cores"`      // 规格算力占比或 AiCore 数
	ForUsage  string       `json:"for_usage"`  // "inference" / "training" / "mixed"
	Labels    map[string]string `json:"labels"`
}

// UnifiedResourcePool 统一资源池管理器
type UnifiedResourcePool struct {
	pools             map[string]ResourcePool
	flavors           map[string]*ResourceFlavor
	allocationRecords map[string]*AllocationRecord // allocationID -> record
	mu                sync.RWMutex
}

// NewUnifiedResourcePool 创建统一资源池管理器
func NewUnifiedResourcePool() *UnifiedResourcePool {
	return &UnifiedResourcePool{
		pools:             make(map[string]ResourcePool),
		flavors:           make(map[string]*ResourceFlavor),
		allocationRecords: make(map[string]*AllocationRecord),
	}
}

// recordAllocation 记录一次分配（供 AllocateFromPool/AllocateAny 成功后调用）
func (urp *UnifiedResourcePool) recordAllocation(poolID string, result *AllocationResult, taskID string) {
	if result == nil {
		return
	}
	ids := make([]string, 0, len(result.Resources))
	for _, r := range result.Resources {
		ids = append(ids, r.ID)
	}
	urp.mu.Lock()
	defer urp.mu.Unlock()
	urp.allocationRecords[result.AllocationID] = &AllocationRecord{
		AllocationID: result.AllocationID,
		PoolID:       poolID,
		ResourceIDs:  ids,
		TaskID:       taskID,
		CreatedAt:    time.Now(),
	}
}

// removeAllocation 移除分配记录（释放时调用）
func (urp *UnifiedResourcePool) removeAllocation(allocationID string) {
	urp.mu.Lock()
	defer urp.mu.Unlock()
	delete(urp.allocationRecords, allocationID)
}

// ListAllocations 列出某池的已分配记录，用于感知已分配资源与剩余
func (urp *UnifiedResourcePool) ListAllocations(poolID string) []*AllocationRecord {
	urp.mu.RLock()
	defer urp.mu.RUnlock()
	var out []*AllocationRecord
	for _, rec := range urp.allocationRecords {
		if rec.PoolID == poolID {
			out = append(out, rec)
		}
	}
	return out
}

// UnregisterPool 注销资源池（仍有分配时拒绝）
func (urp *UnifiedResourcePool) UnregisterPool(poolID string) error {
	urp.mu.Lock()
	defer urp.mu.Unlock()
	if _, ok := urp.pools[poolID]; !ok {
		return ErrPoolNotFound
	}
	for _, rec := range urp.allocationRecords {
		if rec.PoolID == poolID {
			return fmt.Errorf("pool %s still has active allocations", poolID)
		}
	}
	delete(urp.pools, poolID)
	return nil
}

// RegisterPool 注册资源池
func (urp *UnifiedResourcePool) RegisterPool(poolID string, pool ResourcePool) {
	urp.mu.Lock()
	defer urp.mu.Unlock()
	urp.pools[poolID] = pool
}

// RegisterFlavor 注册资源规格（例如 gpu.a100.20g.infer）。
func (urp *UnifiedResourcePool) RegisterFlavor(flavor *ResourceFlavor) {
	if flavor == nil || flavor.ID == "" {
		return
	}
	urp.mu.Lock()
	defer urp.mu.Unlock()
	urp.flavors[flavor.ID] = flavor
}

// GetFlavor 获取规格信息。
func (urp *UnifiedResourcePool) GetFlavor(flavorID string) *ResourceFlavor {
	urp.mu.RLock()
	defer urp.mu.RUnlock()
	return urp.flavors[flavorID]
}

// ListFlavors 列出所有已注册的规格。
func (urp *UnifiedResourcePool) ListFlavors() []*ResourceFlavor {
	urp.mu.RLock()
	defer urp.mu.RUnlock()
	result := make([]*ResourceFlavor, 0, len(urp.flavors))
	for _, flavor := range urp.flavors {
		result = append(result, flavor)
	}
	return result
}

// GetPool 获取资源池
func (urp *UnifiedResourcePool) GetPool(poolID string) ResourcePool {
	urp.mu.RLock()
	defer urp.mu.RUnlock()
	return urp.pools[poolID]
}

// ListPools 列出所有资源池
func (urp *UnifiedResourcePool) ListPools(ctx context.Context) ([]*PoolInfo, error) {
	urp.mu.RLock()
	defer urp.mu.RUnlock()
	
	var pools []*PoolInfo
	for poolID, pool := range urp.pools {
		info, err := pool.GetPoolInfo(ctx)
		if err != nil {
			continue
		}
		info.ID = poolID
		pools = append(pools, info)
	}
	return pools, nil
}

// AllocateFromPool 从指定资源池分配资源
func (urp *UnifiedResourcePool) AllocateFromPool(ctx context.Context, poolID string, req *AllocationRequest) (*AllocationResult, error) {
	// 如果指定了规格但未明确资源大小，则根据规格填充请求
	if req != nil && req.Labels != nil {
		if flavorID, ok := req.Labels["flavor_id"]; ok && flavorID != "" {
			if flavor := urp.GetFlavor(flavorID); flavor != nil {
				if req.ResourceType == "" {
					req.ResourceType = flavor.Type
				}
				if req.MemoryGB == 0 {
					req.MemoryGB = flavor.MemoryGB
				}
				if req.Cores == 0 {
					req.Cores = flavor.Cores
				}
			}
		}
	}
	pool := urp.GetPool(poolID)
	if pool == nil {
		return nil, ErrPoolNotFound
	}
	result, err := pool.AllocateResources(ctx, req)
	if err != nil {
		return nil, err
	}
	result.PoolID = poolID
	urp.recordAllocation(poolID, result, req.TaskID)
	return result, nil
}

type poolEntry struct {
	PoolID string
	Pool   ResourcePool
}

// sortedPools 返回按 ID 排序的资源池快照（调用方不得持锁）
func (urp *UnifiedResourcePool) sortedPools() []poolEntry {
	urp.mu.RLock()
	defer urp.mu.RUnlock()
	out := make([]poolEntry, 0, len(urp.pools))
	for id, p := range urp.pools {
		out = append(out, poolEntry{PoolID: id, Pool: p})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PoolID < out[j].PoolID })
	return out
}

// PoolForResource 查找资源所属的资源池
func (urp *UnifiedResourcePool) PoolForResource(ctx context.Context, resourceID string) (string, ResourcePool, bool) {
	for _, e := range urp.sortedPools() {
		if _, err := e.Pool.GetResource(ctx, resourceID); err == nil {
			return e.PoolID, e.Pool, true
		}
	}
	return "", nil, false
}

// AllocateAny 从任意可用资源池分配资源；支持 ConstrainNodeIDs / ConstrainPoolIDs 限制大池/集群池，
// 以及 ResourceIDs 精确落位（由调度器决策）。
func (urp *UnifiedResourcePool) AllocateAny(ctx context.Context, req *AllocationRequest) (*AllocationResult, error) {
	if req == nil {
		return nil, ErrInvalidRequest
	}
	if len(req.ResourceIDs) > 0 {
		poolID, p, ok := urp.PoolForResource(ctx, req.ResourceIDs[0])
		if !ok {
			return nil, fmt.Errorf("resource not found: %s", req.ResourceIDs[0])
		}
		res, err := p.AllocateResources(ctx, req)
		if err != nil {
			return nil, err
		}
		res.PoolID = poolID
		urp.recordAllocation(poolID, res, req.TaskID)
		return res, nil
	}
	allowed := make(map[string]bool, len(req.ConstrainPoolIDs))
	for _, id := range req.ConstrainPoolIDs {
		allowed[id] = true
	}
	var lastErr error = ErrInsufficientResources
	for _, c := range urp.sortedPools() {
		if len(allowed) > 0 && !allowed[c.PoolID] {
			continue
		}
		info, err := c.Pool.GetPoolInfo(ctx)
		if err != nil || (req.ResourceType != "" && info.Type != req.ResourceType) {
			continue
		}
		res, err := c.Pool.AllocateResources(ctx, req)
		if err != nil {
			lastErr = err
			continue
		}
		res.PoolID = c.PoolID
		urp.recordAllocation(c.PoolID, res, req.TaskID)
		return res, nil
	}
	return nil, lastErr
}

// SetCoreOvercommit 设置所有账本资源池的分时复用算力超卖比例
func (urp *UnifiedResourcePool) SetCoreOvercommit(ratio float64) {
	for _, e := range urp.sortedPools() {
		if lp, ok := e.Pool.(*LedgerPool); ok {
			lp.Ledger.SetCoreOvercommit(ratio)
		}
	}
}

func (urp *UnifiedResourcePool) ledgerOf(resourceID string) *Ledger {
	for _, e := range urp.sortedPools() {
		if lp, ok := e.Pool.(*LedgerPool); ok && lp.Ledger.HasResource(resourceID) {
			return lp.Ledger
		}
	}
	return nil
}

// CordonDevice 隔离/解除隔离设备
func (urp *UnifiedResourcePool) CordonDevice(resourceID string, cordoned bool) error {
	if l := urp.ledgerOf(resourceID); l != nil {
		return l.SetCordon(resourceID, cordoned)
	}
	return fmt.Errorf("resource not found: %s", resourceID)
}

// SetDeviceHealth 设置设备健康状态
func (urp *UnifiedResourcePool) SetDeviceHealth(resourceID, health string) error {
	if l := urp.ledgerOf(resourceID); l != nil {
		return l.SetHealth(resourceID, health)
	}
	return fmt.Errorf("resource not found: %s", resourceID)
}

// SetDeviceLabel 设置设备标签
func (urp *UnifiedResourcePool) SetDeviceLabel(resourceID, key, value string) error {
	if l := urp.ledgerOf(resourceID); l != nil {
		return l.SetDeviceLabel(resourceID, key, value)
	}
	return fmt.Errorf("resource not found: %s", resourceID)
}

// GetAllocationRecord 查询分配记录
func (urp *UnifiedResourcePool) GetAllocationRecord(allocationID string) (*AllocationRecord, bool) {
	urp.mu.RLock()
	defer urp.mu.RUnlock()
	rec, ok := urp.allocationRecords[allocationID]
	return rec, ok
}

// ReleaseAllocation 释放分配的资源
func (urp *UnifiedResourcePool) ReleaseAllocation(ctx context.Context, poolID, allocationID string) error {
	pool := urp.GetPool(poolID)
	if pool == nil {
		return ErrPoolNotFound
	}
	if err := pool.ReleaseResources(ctx, allocationID); err != nil {
		return err
	}
	urp.removeAllocation(allocationID)
	return nil
}

// 实现 ResourcePool 接口，使 UnifiedResourcePool 可以作为 ResourcePool 使用

// GetPoolInfo 获取统一资源池信息（聚合所有子池的信息）
func (urp *UnifiedResourcePool) GetPoolInfo(ctx context.Context) (*PoolInfo, error) {
	urp.mu.RLock()
	defer urp.mu.RUnlock()
	
	info := &PoolInfo{
		ID:   "unified",
		Name: "Unified Resource Pool",
		Type: ResourceTypeGPU, // 默认类型，实际是混合的
	}
	
	for _, pool := range urp.pools {
		poolInfo, err := pool.GetPoolInfo(ctx)
		if err != nil {
			continue
		}
		info.TotalCount += poolInfo.TotalCount
		info.AvailableCount += poolInfo.AvailableCount
		info.AllocatedCount += poolInfo.AllocatedCount
		info.TotalMemory += poolInfo.TotalMemory
		info.UsedMemory += poolInfo.UsedMemory
		info.Nodes = append(info.Nodes, poolInfo.Nodes...)
	}
	
	info.CreatedAt = time.Now()
	info.UpdatedAt = time.Now()
	return info, nil
}

// ListResources 列出所有资源（聚合所有子池的资源）
func (urp *UnifiedResourcePool) ListResources(ctx context.Context, filters map[string]string) ([]*Resource, error) {
	var allResources []*Resource
	seen := make(map[string]bool)
	for _, e := range urp.sortedPools() {
		resources, err := e.Pool.ListResources(ctx, filters)
		if err != nil {
			continue
		}
		for _, r := range resources {
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			allResources = append(allResources, r)
		}
	}
	return allResources, nil
}

// GetResource 获取资源（从所有子池中查找）
func (urp *UnifiedResourcePool) GetResource(ctx context.Context, resourceID string) (*Resource, error) {
	for _, e := range urp.sortedPools() {
		if resource, err := e.Pool.GetResource(ctx, resourceID); err == nil {
			return resource, nil
		}
	}
	return nil, fmt.Errorf("resource not found: %s", resourceID)
}

// AllocateResources 分配资源（自动选择合适的资源池）
func (urp *UnifiedResourcePool) AllocateResources(ctx context.Context, req *AllocationRequest) (*AllocationResult, error) {
	return urp.AllocateAny(ctx, req)
}

// ReleaseResources 释放资源：优先按分配记录定位资源池
func (urp *UnifiedResourcePool) ReleaseResources(ctx context.Context, allocationID string) error {
	if rec, ok := urp.GetAllocationRecord(allocationID); ok {
		return urp.ReleaseAllocation(ctx, rec.PoolID, allocationID)
	}
	for _, e := range urp.sortedPools() {
		if err := e.Pool.ReleaseResources(ctx, allocationID); err == nil {
			urp.removeAllocation(allocationID)
			return nil
		}
	}
	return ErrAllocationNotFound
}

// UpdateResourceState 更新资源状态（从所有子池中查找并更新）
func (urp *UnifiedResourcePool) UpdateResourceState(ctx context.Context, resourceID string, state ResourceState) error {
	for _, e := range urp.sortedPools() {
		if err := e.Pool.UpdateResourceState(ctx, resourceID, state); err == nil {
			return nil
		}
	}
	return fmt.Errorf("resource not found: %s", resourceID)
}

// GetResourceMetrics 获取资源指标（从所有子池中查找）
func (urp *UnifiedResourcePool) GetResourceMetrics(ctx context.Context, resourceID string, duration time.Duration) (*ResourceMetrics, error) {
	for _, e := range urp.sortedPools() {
		if metrics, err := e.Pool.GetResourceMetrics(ctx, resourceID, duration); err == nil {
			return metrics, nil
		}
	}
	return nil, fmt.Errorf("resource not found: %s", resourceID)
}

// 错误定义
var (
	ErrPoolNotFound        = &PoolError{Code: "POOL_NOT_FOUND", Message: "Resource pool not found"}
	ErrInsufficientResources = &PoolError{Code: "INSUFFICIENT_RESOURCES", Message: "Insufficient resources available"}
	ErrInvalidRequest      = &PoolError{Code: "INVALID_REQUEST", Message: "Invalid allocation request"}
	ErrAllocationNotFound  = &PoolError{Code: "ALLOCATION_NOT_FOUND", Message: "Allocation not found"}
)

// PoolError 资源池错误
type PoolError struct {
	Code    string
	Message string
}

func (e *PoolError) Error() string {
	return e.Message
}
