/*
 * Unified Resource Manager - Compute Perception (算力感知)
 *
 * 算力感知：实时感知和监控算力资源的状态、可用性和性能
 * 功能包括：
 * - 算力资源绑定 (Compute Resource Binding)
 * - 算力资源管理 (Compute Resource Management)
 * - 算力资源监控 (Compute Resource Monitoring)
 * - 算力集群设置 (Compute Cluster Settings)
 */

package compute

import (
	"context"
	"fmt"
	"sync"
	"time"

	"flexai/unified-resource-manager/core/cluster"
	"flexai/unified-resource-manager/core/pool"
)

// ComputePerception 算力感知管理器
type ComputePerception struct {
	clusterMgr *cluster.ClusterManager
	resourcePool *pool.UnifiedResourcePool
	
	// 资源状态缓存
	resourceStates map[string]*ResourceState
	mu            sync.RWMutex
	
	// 监控配置
	monitorInterval time.Duration
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

// ResourceState 资源状态
type ResourceState struct {
	ResourceID    string            `json:"resource_id"`
	NodeID        string            `json:"node_id"`
	Type          pool.ResourceType `json:"type"`
	State         pool.ResourceState `json:"state"`
	Health        HealthStatus      `json:"health"`
	Utilization   float64           `json:"utilization"`   // 0-100
	Temperature   float64           `json:"temperature"`   // Celsius
	PowerUsage    float64           `json:"power_usage"`    // Watts
	MemoryUsage   float64           `json:"memory_usage"`  // 0-100
	CoreUsage     float64           `json:"core_usage"`     // 0-100
	LastSeen      time.Time         `json:"last_seen"`
	Labels        map[string]string `json:"labels"`
}

// HealthStatus 健康状态
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusWarning   HealthStatus = "warning"
	HealthStatusCritical  HealthStatus = "critical"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// ClusterSettings 集群设置
type ClusterSettings struct {
	ClusterID      string            `json:"cluster_id"`
	Name           string            `json:"name"`
	AutoDiscovery  bool              `json:"auto_discovery"`
	HealthCheckInterval time.Duration `json:"health_check_interval"`
	ResourceTags   map[string]string `json:"resource_tags"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// NewComputePerception 创建算力感知管理器
func NewComputePerception(clusterMgr *cluster.ClusterManager, resourcePool *pool.UnifiedResourcePool) *ComputePerception {
	ctx, cancel := context.WithCancel(context.Background())
	return &ComputePerception{
		clusterMgr:      clusterMgr,
		resourcePool:   resourcePool,
		resourceStates: make(map[string]*ResourceState),
		monitorInterval: 10 * time.Second,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Start 启动算力感知监控
func (cp *ComputePerception) Start() error {
	cp.wg.Add(1)
	go cp.monitorLoop()
	return nil
}

// Stop 停止算力感知监控
func (cp *ComputePerception) Stop() {
	cp.cancel()
	cp.wg.Wait()
}

// monitorLoop 监控循环
func (cp *ComputePerception) monitorLoop() {
	defer cp.wg.Done()
	
	ticker := time.NewTicker(cp.monitorInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-cp.ctx.Done():
			return
		case <-ticker.C:
			cp.updateResourceStates()
		}
	}
}

// updateResourceStates 更新资源状态
func (cp *ComputePerception) updateResourceStates() {
	ctx := context.Background()
	
	// 从资源池获取所有资源
	resources, err := cp.resourcePool.ListResources(ctx, nil)
	if err != nil {
		return
	}
	
	cp.mu.Lock()
	defer cp.mu.Unlock()
	
	for _, resource := range resources {
		state := cp.getOrCreateResourceState(resource.ID)
		state.ResourceID = resource.ID
		state.NodeID = resource.NodeID
		state.Type = resource.Type
		state.State = resource.State
		state.LastSeen = time.Now()
		
		// 计算利用率
		if resource.TotalMemory > 0 {
			state.MemoryUsage = float64(resource.UsedMemory) / float64(resource.TotalMemory) * 100
		}
		if resource.TotalCores > 0 {
			state.CoreUsage = float64(resource.UsedCores) / float64(resource.TotalCores) * 100
		}
		state.Utilization = (state.MemoryUsage + state.CoreUsage) / 2
		
		// 评估健康状态
		state.Health = cp.evaluateHealth(state)
		
		cp.resourceStates[resource.ID] = state
	}
}

// getOrCreateResourceState 获取或创建资源状态
func (cp *ComputePerception) getOrCreateResourceState(resourceID string) *ResourceState {
	if state, ok := cp.resourceStates[resourceID]; ok {
		return state
	}
	return &ResourceState{
		ResourceID: resourceID,
		Health:     HealthStatusUnknown,
		Labels:     make(map[string]string),
	}
}

// evaluateHealth 评估资源健康状态
func (cp *ComputePerception) evaluateHealth(state *ResourceState) HealthStatus {
	// 检查资源是否超时
	if time.Since(state.LastSeen) > 60*time.Second {
		return HealthStatusCritical
	}
	
	// 检查利用率
	if state.Utilization > 95 {
		return HealthStatusWarning
	}
	
	// 检查温度（如果有）
	if state.Temperature > 85 {
		return HealthStatusWarning
	}
	if state.Temperature > 95 {
		return HealthStatusCritical
	}
	
	return HealthStatusHealthy
}

// BindResource 绑定资源到任务
func (cp *ComputePerception) BindResource(ctx context.Context, resourceID string, taskID string) error {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	
	state, ok := cp.resourceStates[resourceID]
	if !ok {
		return fmt.Errorf("resource not found: %s", resourceID)
	}
	
	if state.State != pool.StateAvailable {
		return fmt.Errorf("resource not available: %s, state: %s", resourceID, state.State)
	}
	
	// 添加绑定标签
	if state.Labels == nil {
		state.Labels = make(map[string]string)
	}
	state.Labels["task_id"] = taskID
	state.Labels["bound_at"] = time.Now().Format(time.RFC3339)
	
	return nil
}

// UnbindResource 解绑资源
func (cp *ComputePerception) UnbindResource(ctx context.Context, resourceID string) error {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	
	state, ok := cp.resourceStates[resourceID]
	if !ok {
		return fmt.Errorf("resource not found: %s", resourceID)
	}
	
	// 移除绑定标签
	if state.Labels != nil {
		delete(state.Labels, "task_id")
		delete(state.Labels, "bound_at")
	}
	
	return nil
}

// GetResourceState 获取资源状态
func (cp *ComputePerception) GetResourceState(resourceID string) (*ResourceState, error) {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	
	state, ok := cp.resourceStates[resourceID]
	if !ok {
		return nil, fmt.Errorf("resource not found: %s", resourceID)
	}
	
	return state, nil
}

// ListResourceStates 列出所有资源状态
func (cp *ComputePerception) ListResourceStates(filters map[string]string) []*ResourceState {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	
	var states []*ResourceState
	for _, state := range cp.resourceStates {
		if cp.matchFilters(state, filters) {
			states = append(states, state)
		}
	}
	
	return states
}

// matchFilters 匹配过滤器
func (cp *ComputePerception) matchFilters(state *ResourceState, filters map[string]string) bool {
	if len(filters) == 0 {
		return true
	}
	
	for k, v := range filters {
		switch k {
		case "type":
			if string(state.Type) != v {
				return false
			}
		case "state":
			if string(state.State) != v {
				return false
			}
		case "health":
			if string(state.Health) != v {
				return false
			}
		case "node_id":
			if state.NodeID != v {
				return false
			}
		}
	}
	
	return true
}

// GetClusterSettings 获取集群设置
func (cp *ComputePerception) GetClusterSettings() *ClusterSettings {
	return &ClusterSettings{
		ClusterID:         "default-cluster",
		Name:              "Default Cluster",
		AutoDiscovery:     true,
		HealthCheckInterval: cp.monitorInterval,
		ResourceTags:      make(map[string]string),
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
}

// UpdateClusterSettings 更新集群设置
func (cp *ComputePerception) UpdateClusterSettings(settings *ClusterSettings) error {
	if settings.HealthCheckInterval > 0 {
		cp.monitorInterval = settings.HealthCheckInterval
	}
	return nil
}
