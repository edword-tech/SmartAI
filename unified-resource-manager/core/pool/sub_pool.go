/*
 * 子池（大池内切分）：在大池/物理池上按配额或切分规则再划分逻辑子池
 * - 用于租户隔离、按业务线切分、或按规格切分（vGPU 等）
 */

package pool

import (
	"fmt"
	"sync"
	"time"
)

// SubPoolSpec 子池规格
type SubPoolSpec struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	// ParentClusterPoolID 所属集群池（大池）ID；与 ParentPoolID 二选一
	ParentClusterPoolID string `json:"parent_cluster_pool_id,omitempty"`
	// ParentPoolID 所属物理池 ID
	ParentPoolID string `json:"parent_pool_id,omitempty"`
	// QuotaMaxCount 子池最大 GPU/NPU 卡数，0 表示不限制
	QuotaMaxCount int `json:"quota_max_count,omitempty"`
	// QuotaMaxMemoryGB 子池最大显存（GB），0 表示不限制
	QuotaMaxMemoryGB int64 `json:"quota_max_memory_gb,omitempty"`
	// SliceConfigID 关联的切分配置 ID（算力质量中的 SlicingConfig），实现池内按规格切分
	SliceConfigID string            `json:"slice_config_id,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// SubPoolUsage 子池已用配额（用于分配时校验）
type SubPoolUsage struct {
	Count     int   `json:"count"`
	MemoryGB  int64 `json:"memory_gb"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SubPoolManager 子池管理 + 配额追踪
type SubPoolManager struct {
	specs map[string]*SubPoolSpec
	usage map[string]*SubPoolUsage
	mu    sync.RWMutex
}

// NewSubPoolManager 创建子池管理器
func NewSubPoolManager() *SubPoolManager {
	return &SubPoolManager{
		specs: make(map[string]*SubPoolSpec),
		usage: make(map[string]*SubPoolUsage),
	}
}

// Create 创建子池
func (m *SubPoolManager) Create(spec *SubPoolSpec) error {
	if spec == nil || spec.ID == "" || spec.Name == "" {
		return fmt.Errorf("invalid sub pool spec")
	}
	if spec.ParentClusterPoolID == "" && spec.ParentPoolID == "" {
		return fmt.Errorf("sub pool must have parent_cluster_pool_id or parent_pool_id")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	spec.CreatedAt = now
	spec.UpdatedAt = now
	m.specs[spec.ID] = spec
	m.usage[spec.ID] = &SubPoolUsage{UpdatedAt: now}
	return nil
}

// Get 获取子池规格
func (m *SubPoolManager) Get(id string) (*SubPoolSpec, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	spec, ok := m.specs[id]
	if !ok {
		return nil, fmt.Errorf("sub pool not found: %s", id)
	}
	return spec, nil
}

// List 列出所有子池
func (m *SubPoolManager) List() []*SubPoolSpec {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*SubPoolSpec, 0, len(m.specs))
	for _, s := range m.specs {
		out = append(out, s)
	}
	return out
}

// ListByParent 按父池 ID 列出子池
func (m *SubPoolManager) ListByParent(clusterPoolID string, physicalPoolID string) []*SubPoolSpec {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*SubPoolSpec
	for _, s := range m.specs {
		if clusterPoolID != "" && s.ParentClusterPoolID == clusterPoolID {
			out = append(out, s)
		}
		if physicalPoolID != "" && s.ParentPoolID == physicalPoolID {
			out = append(out, s)
		}
	}
	return out
}

// Delete 删除子池
func (m *SubPoolManager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.specs[id]; !ok {
		return fmt.Errorf("sub pool not found: %s", id)
	}
	delete(m.specs, id)
	delete(m.usage, id)
	return nil
}

// GetUsage 获取子池当前用量
func (m *SubPoolManager) GetUsage(subPoolID string) (*SubPoolUsage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.usage[subPoolID]
	if !ok {
		return nil, fmt.Errorf("sub pool not found: %s", subPoolID)
	}
	return u, nil
}

// AddUsage 增加子池用量（分配成功后调用）
func (m *SubPoolManager) AddUsage(subPoolID string, count int, memoryGB int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	spec, ok := m.specs[subPoolID]
	if !ok {
		return fmt.Errorf("sub pool not found: %s", subPoolID)
	}
	u := m.usage[subPoolID]
	if u == nil {
		u = &SubPoolUsage{}
		m.usage[subPoolID] = u
	}
	u.Count += count
	u.MemoryGB += memoryGB
	u.UpdatedAt = time.Now()
	_ = spec
	return nil
}

// SubUsage 减少子池用量（释放后调用）
func (m *SubPoolManager) SubUsage(subPoolID string, count int, memoryGB int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.usage[subPoolID]
	if !ok {
		return nil
	}
	u.Count -= count
	u.MemoryGB -= memoryGB
	if u.Count < 0 {
		u.Count = 0
	}
	if u.MemoryGB < 0 {
		u.MemoryGB = 0
	}
	u.UpdatedAt = time.Now()
	return nil
}

// CheckQuota 检查子池配额是否允许此次分配
func (m *SubPoolManager) CheckQuota(subPoolID string, addCount int, addMemoryGB int64) (ok bool, err error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	spec, ok := m.specs[subPoolID]
	if !ok {
		return false, fmt.Errorf("sub pool not found: %s", subPoolID)
	}
	u := m.usage[subPoolID]
	if u == nil {
		u = &SubPoolUsage{}
	}
	afterCount := u.Count + addCount
	afterMemory := u.MemoryGB + addMemoryGB
	if spec.QuotaMaxCount > 0 && afterCount > spec.QuotaMaxCount {
		return false, nil
	}
	if spec.QuotaMaxMemoryGB > 0 && afterMemory > spec.QuotaMaxMemoryGB {
		return false, nil
	}
	return true, nil
}
