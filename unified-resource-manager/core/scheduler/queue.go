/*
 * Unified Resource Manager - Scheduling Queues
 *
 * 队列（参照 Volcano Queue / Kueue ClusterQueue）：
 * - Weight：按主导资源份额 (DRF) / 权重 做队列间公平调度
 * - Guaranteed：保障配额，低于保障的队列可回收 (reclaim) 其他队列借用的资源
 * - Capability：上限配额（0 表示不限），空闲时允许借用到上限
 */

package scheduler

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"flexai/unified-resource-manager/core/pool"
)

// DefaultQueue 默认队列
const DefaultQueue = "default"

// ResourceList 资源量
type ResourceList struct {
	GPU      int   `json:"gpu"`
	NPU      int   `json:"npu"`
	MemoryGB int64 `json:"memory_gb"`
}

func (r ResourceList) add(o ResourceList) ResourceList {
	return ResourceList{r.GPU + o.GPU, r.NPU + o.NPU, r.MemoryGB + o.MemoryGB}
}

// fitsWithin 判断 r 是否不超过 limit（limit 的 0 字段视为不限）
func (r ResourceList) fitsWithin(limit ResourceList) bool {
	if limit.GPU > 0 && r.GPU > limit.GPU {
		return false
	}
	if limit.NPU > 0 && r.NPU > limit.NPU {
		return false
	}
	if limit.MemoryGB > 0 && r.MemoryGB > limit.MemoryGB {
		return false
	}
	return true
}

func (r ResourceList) isZero() bool { return r.GPU == 0 && r.NPU == 0 && r.MemoryGB == 0 }

// Queue 调度队列
type Queue struct {
	Name        string       `json:"name"`
	TenantID    string       `json:"tenant_id,omitempty"`
	Weight      int          `json:"weight"`
	Guaranteed  ResourceList `json:"guaranteed"`
	Capability  ResourceList `json:"capability"`
	Reclaimable bool         `json:"reclaimable"`
	CreatedAt   time.Time    `json:"created_at"`

	// 运行态（由调度器计算）
	Allocated     ResourceList `json:"allocated"`
	Pending       int          `json:"pending"`
	Running       int          `json:"running"`
	DominantShare float64      `json:"dominant_share"`
}

// taskRequest 计算任务占用资源量（按请求规格，已分配时按实际卡数）
func taskRequest(t *Task, count int) ResourceList {
	var r ResourceList
	if t.ResourceReq == nil {
		return r
	}
	if count <= 0 {
		count = t.ResourceReq.Count
	}
	switch t.ResourceReq.ResourceType {
	case pool.ResourceTypeGPU:
		r.GPU = count
	case pool.ResourceTypeNPU:
		r.NPU = count
	}
	for _, s := range t.Slices {
		r.MemoryGB += s.MemoryGB
	}
	if len(t.Slices) == 0 {
		r.MemoryGB = t.ResourceReq.MemoryGB * int64(count)
	}
	return r
}

// QueueManager 队列管理
type QueueManager struct {
	queues map[string]*Queue
	mu     sync.RWMutex
}

// NewQueueManager 创建队列管理器（自带 default 队列）
func NewQueueManager() *QueueManager {
	qm := &QueueManager{queues: make(map[string]*Queue)}
	_ = qm.Upsert(&Queue{Name: DefaultQueue, Weight: 1, Reclaimable: true})
	return qm
}

// Upsert 创建或更新队列
func (qm *QueueManager) Upsert(q *Queue) error {
	if q == nil || q.Name == "" {
		return fmt.Errorf("queue name required")
	}
	if q.Weight <= 0 {
		q.Weight = 1
	}
	if !q.Capability.isZero() && !q.Guaranteed.fitsWithin(q.Capability) {
		return fmt.Errorf("guaranteed exceeds capability")
	}
	qm.mu.Lock()
	defer qm.mu.Unlock()
	if old, ok := qm.queues[q.Name]; ok {
		q.CreatedAt = old.CreatedAt
		q.Allocated, q.Pending, q.Running, q.DominantShare = old.Allocated, old.Pending, old.Running, old.DominantShare
	} else {
		q.CreatedAt = time.Now()
	}
	qm.queues[q.Name] = q
	return nil
}

// Delete 删除队列
func (qm *QueueManager) Delete(name string) error {
	if name == DefaultQueue {
		return fmt.Errorf("default queue cannot be deleted")
	}
	qm.mu.Lock()
	defer qm.mu.Unlock()
	q, ok := qm.queues[name]
	if !ok {
		return fmt.Errorf("queue not found: %s", name)
	}
	if q.Running > 0 || q.Pending > 0 {
		return fmt.Errorf("queue %s still has tasks", name)
	}
	delete(qm.queues, name)
	return nil
}

// Get 获取队列副本
func (qm *QueueManager) Get(name string) (*Queue, bool) {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	q, ok := qm.queues[name]
	if !ok {
		return nil, false
	}
	cp := *q
	return &cp, true
}

// List 列出队列副本
func (qm *QueueManager) List() []*Queue {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	out := make([]*Queue, 0, len(qm.queues))
	for _, q := range qm.queues {
		cp := *q
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ResolveQueue 为任务确定队列：显式指定 > 与租户同名/归属租户的队列 > default
func (qm *QueueManager) ResolveQueue(t *Task) string {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	if t.Queue != "" {
		if _, ok := qm.queues[t.Queue]; ok {
			return t.Queue
		}
	}
	if t.TenantID != "" {
		if _, ok := qm.queues[t.TenantID]; ok {
			return t.TenantID
		}
		names := make([]string, 0)
		for name, q := range qm.queues {
			if q.TenantID == t.TenantID {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		if len(names) > 0 {
			return names[0]
		}
	}
	return DefaultQueue
}

// recompute 根据任务列表重算队列用量与主导份额
func (qm *QueueManager) recompute(tasks map[string]*Task, cluster ResourceList) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	for _, q := range qm.queues {
		q.Allocated, q.Pending, q.Running = ResourceList{}, 0, 0
	}
	for _, t := range tasks {
		q, ok := qm.queues[t.Queue]
		if !ok {
			continue
		}
		switch t.Status {
		case TaskStatusRunning:
			q.Running++
			q.Allocated = q.Allocated.add(taskRequest(t, len(t.Slices)))
		case TaskStatusPending:
			q.Pending++
		}
	}
	for _, q := range qm.queues {
		q.DominantShare = dominantShare(q.Allocated, cluster) / float64(q.Weight)
	}
}

func dominantShare(used, total ResourceList) float64 {
	share := 0.0
	if total.GPU > 0 {
		share = maxF(share, float64(used.GPU)/float64(total.GPU))
	}
	if total.NPU > 0 {
		share = maxF(share, float64(used.NPU)/float64(total.NPU))
	}
	if total.MemoryGB > 0 {
		share = maxF(share, float64(used.MemoryGB)/float64(total.MemoryGB))
	}
	return share
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// share 返回队列当前加权主导份额
func (qm *QueueManager) share(name string) float64 {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	if q, ok := qm.queues[name]; ok {
		return q.DominantShare
	}
	return 0
}

// admit 队列准入：分配后不得超过 Capability
func (qm *QueueManager) admit(name string, req ResourceList) error {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	q, ok := qm.queues[name]
	if !ok {
		return fmt.Errorf("queue %s not found", name)
	}
	if !q.Capability.isZero() && !q.Allocated.add(req).fitsWithin(q.Capability) {
		return fmt.Errorf("queue %s capability exceeded (allocated %+v + request %+v > %+v)", name, q.Allocated, req, q.Capability)
	}
	return nil
}

// assume 在一个调度周期内累加队列用量
func (qm *QueueManager) assume(name string, req ResourceList) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	if q, ok := qm.queues[name]; ok {
		q.Allocated = q.Allocated.add(req)
	}
}

// underGuarantee 队列在加上 req 后是否仍在保障配额内（可发起回收）
func (qm *QueueManager) underGuarantee(name string, req ResourceList) bool {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	q, ok := qm.queues[name]
	if !ok || q.Guaranteed.isZero() {
		return false
	}
	return q.Allocated.add(req).fitsWithin(q.Guaranteed)
}

// overGuarantee 队列是否借用了超出保障的资源（其任务可被回收）
func (qm *QueueManager) overGuarantee(name string) bool {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	q, ok := qm.queues[name]
	if !ok || !q.Reclaimable {
		return false
	}
	return !q.Allocated.fitsWithin(q.Guaranteed) || q.Guaranteed.isZero()
}
