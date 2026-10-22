/*
 * 集群资源池（大池）：将多节点 GPU 聚合成一个逻辑大池，用于大模型集群调度
 * - 支持从大池分配多卡给单个大模型集群（cluster job）
 * - 支持在大池内再切分子池（见 sub_pool.go）
 */

package pool

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ClusterPoolSpec 集群池（大池）规格：聚合哪些节点/物理池
type ClusterPoolSpec struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Type        ResourceType     `json:"type"` // gpu / npu
	// NodeIDs 纳入此大池的节点 ID 列表；分配时仅从这些节点选资源
	NodeIDs []string `json:"node_ids"`
	// PoolIDs 纳入此大池的物理池 ID；为空则表示从同类型所有池按 NodeIDs 约束选
	PoolIDs []string          `json:"pool_ids,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// ClusterPoolManager 集群池管理：创建/列出/删除大池，聚合视图
type ClusterPoolManager struct {
	specs map[string]*ClusterPoolSpec
	mu    sync.RWMutex
}

// NewClusterPoolManager 创建集群池管理器
func NewClusterPoolManager() *ClusterPoolManager {
	return &ClusterPoolManager{specs: make(map[string]*ClusterPoolSpec)}
}

// Create 创建集群池
func (m *ClusterPoolManager) Create(spec *ClusterPoolSpec) error {
	if spec == nil || spec.ID == "" || spec.Name == "" {
		return fmt.Errorf("invalid cluster pool spec")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	spec.CreatedAt = now
	spec.UpdatedAt = now
	m.specs[spec.ID] = spec
	return nil
}

// Get 获取集群池规格
func (m *ClusterPoolManager) Get(id string) (*ClusterPoolSpec, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	spec, ok := m.specs[id]
	if !ok {
		return nil, fmt.Errorf("cluster pool not found: %s", id)
	}
	return spec, nil
}

// List 列出所有集群池
func (m *ClusterPoolManager) List() []*ClusterPoolSpec {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*ClusterPoolSpec, 0, len(m.specs))
	for _, s := range m.specs {
		out = append(out, s)
	}
	return out
}

// Update 更新集群池
func (m *ClusterPoolManager) Update(spec *ClusterPoolSpec) error {
	if spec == nil || spec.ID == "" {
		return fmt.Errorf("invalid cluster pool spec")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.specs[spec.ID]; !ok {
		return fmt.Errorf("cluster pool not found: %s", spec.ID)
	}
	spec.UpdatedAt = time.Now()
	m.specs[spec.ID] = spec
	return nil
}

// Delete 删除集群池
func (m *ClusterPoolManager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.specs[id]; !ok {
		return fmt.Errorf("cluster pool not found: %s", id)
	}
	delete(m.specs, id)
	return nil
}

// GetPoolInfoFromUnified 根据统一资源池与集群池规格，聚合出该大池的 PoolInfo（仅统计指定节点/池内资源）
func (m *ClusterPoolManager) GetPoolInfoFromUnified(ctx context.Context, spec *ClusterPoolSpec, unified *UnifiedResourcePool) (*PoolInfo, error) {
	if spec == nil {
		return nil, fmt.Errorf("nil cluster pool spec")
	}
	info := &PoolInfo{
		ID:        spec.ID,
		Name:      spec.Name,
		Type:      spec.Type,
		Nodes:     spec.NodeIDs,
		Labels:    spec.Labels,
		CreatedAt: spec.CreatedAt,
		UpdatedAt: spec.UpdatedAt,
	}
	nodeSet := make(map[string]bool)
	for _, n := range spec.NodeIDs {
		nodeSet[n] = true
	}
	unified.mu.RLock()
	pools := make(map[string]ResourcePool)
	for pid, p := range unified.pools {
		if len(spec.PoolIDs) > 0 {
			found := false
			for _, id := range spec.PoolIDs {
				if id == pid {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		pools[pid] = p
	}
	unified.mu.RUnlock()
	for _, pool := range pools {
		poolInfo, err := pool.GetPoolInfo(ctx)
		if err != nil || poolInfo.Type != spec.Type {
			continue
		}
		all, _ := pool.ListResources(ctx, nil)
		for _, r := range all {
			if len(nodeSet) > 0 && !nodeSet[r.NodeID] {
				continue
			}
			info.TotalCount++
			info.TotalMemory += r.TotalMemory
			info.UsedMemory += r.UsedMemory
			if r.State == StateAvailable {
				info.AvailableCount++
			} else {
				info.AllocatedCount++
			}
		}
	}
	return info, nil
}
