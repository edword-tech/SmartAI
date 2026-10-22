/*
 * Unified Resource Manager - Compute Quality (算力质量)
 *
 * 算力质量：管理和控制算力资源的服务质量
 * 功能包括：
 * - 算力切分设置 (Compute Slicing Settings)
 * - 算力独占设置 (Compute Exclusive Use Settings)
 * - 算力资源分配 (Compute Resource Allocation)
 * - 基础算力池进出 (Basic Compute Pool Ingress/Egress)
 */

package compute

import (
	"context"
	"fmt"
	"sync"
	"time"

	"flexai/unified-resource-manager/core/pool"
)

// ComputeQuality 算力质量管理器
type ComputeQuality struct {
	resourcePool *pool.UnifiedResourcePool
	
	// 切分配置
	slicingConfigs map[string]*SlicingConfig
	// 独占配置
	exclusiveConfigs map[string]*ExclusiveConfig
	// 分配策略
	allocationStrategies map[string]*AllocationStrategy
	
	mu sync.RWMutex
}

// SlicingConfig 切分配置
type SlicingConfig struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	ResourceType pool.ResourceType `json:"resource_type"`
	SliceSize   SliceSize         `json:"slice_size"`
	MaxSlices   int               `json:"max_slices"`  // 每个资源最大切分数
	Isolation   bool              `json:"isolation"`    // 是否隔离
	Labels      map[string]string `json:"labels"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// SliceSize 切分大小
type SliceSize struct {
	MemoryGB int64 `json:"memory_gb"`
	Cores    int   `json:"cores"`
}

// ExclusiveConfig 独占配置
type ExclusiveConfig struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	ResourceType pool.ResourceType `json:"resource_type"`
	Priority    int               `json:"priority"`     // 优先级阈值
	MaxDuration time.Duration     `json:"max_duration"` // 最大独占时长
	Labels      map[string]string `json:"labels"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// AllocationStrategy 分配策略
type AllocationStrategy struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Strategy    string            `json:"strategy"`     // binpack/spread/fair
	Weight      map[string]float64 `json:"weight"`     // 资源类型权重
	MinQuality  float64           `json:"min_quality"`  // 最低质量要求
	Labels      map[string]string `json:"labels"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// PoolIngressEgress 算力池进出管理
type PoolIngressEgress struct {
	PoolID      string            `json:"pool_id"`
	Action      string            `json:"action"`      // ingress/egress
	ResourceID  string            `json:"resource_id"`
	Reason      string            `json:"reason"`
	Timestamp   time.Time         `json:"timestamp"`
	Labels      map[string]string `json:"labels"`
}

// NewComputeQuality 创建算力质量管理器
func NewComputeQuality(resourcePool *pool.UnifiedResourcePool) *ComputeQuality {
	return &ComputeQuality{
		resourcePool:         resourcePool,
		slicingConfigs:       make(map[string]*SlicingConfig),
		exclusiveConfigs:     make(map[string]*ExclusiveConfig),
		allocationStrategies: make(map[string]*AllocationStrategy),
	}
}

// RegisterSlicingConfig 注册切分配置
func (cq *ComputeQuality) RegisterSlicingConfig(config *SlicingConfig) error {
	if config == nil || config.ID == "" {
		return fmt.Errorf("invalid slicing config")
	}
	
	cq.mu.Lock()
	defer cq.mu.Unlock()
	
	now := time.Now()
	if config.CreatedAt.IsZero() {
		config.CreatedAt = now
	}
	config.UpdatedAt = now
	
	cq.slicingConfigs[config.ID] = config
	return nil
}

// GetSlicingConfig 获取切分配置
func (cq *ComputeQuality) GetSlicingConfig(configID string) (*SlicingConfig, error) {
	cq.mu.RLock()
	defer cq.mu.RUnlock()
	
	config, ok := cq.slicingConfigs[configID]
	if !ok {
		return nil, fmt.Errorf("slicing config not found: %s", configID)
	}
	
	return config, nil
}

// ListSlicingConfigs 列出所有切分配置
func (cq *ComputeQuality) ListSlicingConfigs() []*SlicingConfig {
	cq.mu.RLock()
	defer cq.mu.RUnlock()
	
	configs := make([]*SlicingConfig, 0, len(cq.slicingConfigs))
	for _, config := range cq.slicingConfigs {
		configs = append(configs, config)
	}
	
	return configs
}

// RegisterExclusiveConfig 注册独占配置
func (cq *ComputeQuality) RegisterExclusiveConfig(config *ExclusiveConfig) error {
	if config == nil || config.ID == "" {
		return fmt.Errorf("invalid exclusive config")
	}
	
	cq.mu.Lock()
	defer cq.mu.Unlock()
	
	now := time.Now()
	if config.CreatedAt.IsZero() {
		config.CreatedAt = now
	}
	config.UpdatedAt = now
	
	cq.exclusiveConfigs[config.ID] = config
	return nil
}

// GetExclusiveConfig 获取独占配置
func (cq *ComputeQuality) GetExclusiveConfig(configID string) (*ExclusiveConfig, error) {
	cq.mu.RLock()
	defer cq.mu.RUnlock()
	
	config, ok := cq.exclusiveConfigs[configID]
	if !ok {
		return nil, fmt.Errorf("exclusive config not found: %s", configID)
	}
	
	return config, nil
}

// ListExclusiveConfigs 列出所有独占配置
func (cq *ComputeQuality) ListExclusiveConfigs() []*ExclusiveConfig {
	cq.mu.RLock()
	defer cq.mu.RUnlock()
	out := make([]*ExclusiveConfig, 0, len(cq.exclusiveConfigs))
	for _, c := range cq.exclusiveConfigs {
		out = append(out, c)
	}
	return out
}

// CheckExclusiveEligibility 检查是否满足独占条件
func (cq *ComputeQuality) CheckExclusiveEligibility(configID string, priority int) (bool, error) {
	config, err := cq.GetExclusiveConfig(configID)
	if err != nil {
		return false, err
	}
	
	return priority >= config.Priority, nil
}

// RegisterAllocationStrategy 注册分配策略
func (cq *ComputeQuality) RegisterAllocationStrategy(strategy *AllocationStrategy) error {
	if strategy == nil || strategy.ID == "" {
		return fmt.Errorf("invalid allocation strategy")
	}
	
	cq.mu.Lock()
	defer cq.mu.Unlock()
	
	now := time.Now()
	if strategy.CreatedAt.IsZero() {
		strategy.CreatedAt = now
	}
	strategy.UpdatedAt = now
	
	cq.allocationStrategies[strategy.ID] = strategy
	return nil
}

// GetAllocationStrategy 获取分配策略
func (cq *ComputeQuality) GetAllocationStrategy(strategyID string) (*AllocationStrategy, error) {
	cq.mu.RLock()
	defer cq.mu.RUnlock()
	
	strategy, ok := cq.allocationStrategies[strategyID]
	if !ok {
		return nil, fmt.Errorf("allocation strategy not found: %s", strategyID)
	}
	
	return strategy, nil
}

// ListAllocationStrategies 列出所有分配策略
func (cq *ComputeQuality) ListAllocationStrategies() []*AllocationStrategy {
	cq.mu.RLock()
	defer cq.mu.RUnlock()
	out := make([]*AllocationStrategy, 0, len(cq.allocationStrategies))
	for _, s := range cq.allocationStrategies {
		out = append(out, s)
	}
	return out
}

// AllocateWithQuality 按质量要求分配资源
func (cq *ComputeQuality) AllocateWithQuality(ctx context.Context, req *pool.AllocationRequest, strategyID string) (*pool.AllocationResult, error) {
	strategy, err := cq.GetAllocationStrategy(strategyID)
	if err != nil {
		return nil, err
	}
	
	// 获取可用资源
	resources, err := cq.resourcePool.ListResources(ctx, map[string]string{
		"state": string(pool.StateAvailable),
	})
	if err != nil {
		return nil, err
	}
	
	// 按策略过滤资源
	filteredResources := cq.filterByQuality(resources, strategy)
	if len(filteredResources) == 0 {
		return nil, fmt.Errorf("no resources meet quality requirements")
	}
	
	// 使用资源池分配
	return cq.resourcePool.AllocateResources(ctx, req)
}

// filterByQuality 按质量过滤资源
func (cq *ComputeQuality) filterByQuality(resources []*pool.Resource, strategy *AllocationStrategy) []*pool.Resource {
	var filtered []*pool.Resource
	
	for _, resource := range resources {
		// 计算资源质量分数
		quality := cq.calculateQuality(resource)
		
		if quality >= strategy.MinQuality {
			filtered = append(filtered, resource)
		}
	}
	
	return filtered
}

// calculateQuality 计算资源质量分数
func (cq *ComputeQuality) calculateQuality(resource *pool.Resource) float64 {
	// 简单的质量计算：基于可用性和利用率
	availability := 1.0
	if resource.State != pool.StateAvailable {
		availability = 0.0
	}
	
	memoryUtilization := 0.0
	if resource.TotalMemory > 0 {
		memoryUtilization = float64(resource.UsedMemory) / float64(resource.TotalMemory)
	}
	
	coreUtilization := 0.0
	if resource.TotalCores > 0 {
		coreUtilization = float64(resource.UsedCores) / float64(resource.TotalCores)
	}
	
	// 质量 = 可用性 * (1 - 平均利用率)
	avgUtilization := (memoryUtilization + coreUtilization) / 2
	quality := availability * (1 - avgUtilization)
	
	return quality
}

// RecordPoolIngress 记录算力池进入
func (cq *ComputeQuality) RecordPoolIngress(poolID string, resourceID string, reason string) *PoolIngressEgress {
	return &PoolIngressEgress{
		PoolID:     poolID,
		Action:     "ingress",
		ResourceID: resourceID,
		Reason:     reason,
		Timestamp:  time.Now(),
		Labels:     make(map[string]string),
	}
}

// RecordPoolEgress 记录算力池退出
func (cq *ComputeQuality) RecordPoolEgress(poolID string, resourceID string, reason string) *PoolIngressEgress {
	return &PoolIngressEgress{
		PoolID:     poolID,
		Action:     "egress",
		ResourceID: resourceID,
		Reason:     reason,
		Timestamp:  time.Now(),
		Labels:     make(map[string]string),
	}
}
