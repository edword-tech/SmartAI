/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2024-2025. All rights reserved.
 * Unified Resource Manager - Intelligent Scheduler
 */

package scheduler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"flexai/unified-resource-manager/core/pool"
)

// TaskType 任务类型
type TaskType string

const (
	TaskTypeInference TaskType = "inference"
	TaskTypeTraining  TaskType = "training"
)

// TaskPriority 任务优先级
type TaskPriority int

const (
	PriorityLow      TaskPriority = 1
	PriorityNormal   TaskPriority = 5
	PriorityHigh     TaskPriority = 8
	PriorityCritical TaskPriority = 10
)

// Task 任务定义
type Task struct {
	ID               string                  `json:"id"`
	Name             string                  `json:"name"`
	Type             TaskType                `json:"type"`
	Priority         TaskPriority            `json:"priority"`
	ResourceReq      *pool.AllocationRequest `json:"resource_req"`
	Status           TaskStatus              `json:"status"`
	ProjectID        string                  `json:"project_id,omitempty"`
	ModelID          string                  `json:"model_id,omitempty"`
	InferenceBackend string                  `json:"inference_backend,omitempty"` // 空/gpu/nvidia_nim
	TenantID         string                  `json:"tenant_id,omitempty"`
	Queue            string                  `json:"queue,omitempty"`
	// GangGroup 同组任务（类似 PodGroup）全部可调度时才一起启动；MinMember 为最少成员数
	GangGroup string `json:"gang_group,omitempty"`
	MinMember int    `json:"min_member,omitempty"`
	// MinCount 弹性任务的最少卡数：资源不足时以 [MinCount, Count] 区间降级启动，空闲后自动扩容
	MinCount int `json:"min_count,omitempty"`
	// NodeSelector 节点标签选择器
	NodeSelector map[string]string `json:"node_selector,omitempty"`
	// NonPreemptible 为 true 时不会被抢占
	NonPreemptible bool `json:"non_preemptible,omitempty"`
	// ExpectedDurationSeconds 预计运行时长，>0 时到期自动完成（用于仿真与批处理作业）
	ExpectedDurationSeconds int `json:"expected_duration_seconds,omitempty"`

	CreatedAt       time.Time              `json:"created_at"`
	StartedAt       *time.Time             `json:"started_at,omitempty"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
	AllocationID    string                 `json:"allocation_id,omitempty"`
	PoolID          string                 `json:"pool_id,omitempty"`
	Slices          []pool.AllocationSlice `json:"slices,omitempty"`
	Nodes           []string               `json:"nodes,omitempty"`
	RetryCount      int                    `json:"retry_count"`
	PreemptionCount int                    `json:"preemption_count"`
	Message         string                 `json:"message,omitempty"`
	Events          []TaskEvent            `json:"events,omitempty"`
	Labels          map[string]string      `json:"labels"`
}

// TaskEvent 任务事件
type TaskEvent struct {
	Time    time.Time `json:"time"`
	Type    string    `json:"type"`
	Message string    `json:"message"`
}

const maxTaskEvents = 30

func (t *Task) addEvent(now time.Time, typ, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	t.Events = append(t.Events, TaskEvent{Time: now, Type: typ, Message: msg})
	if len(t.Events) > maxTaskEvents {
		t.Events = t.Events[len(t.Events)-maxTaskEvents:]
	}
}

// TaskStatus 任务状态
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusScheduled TaskStatus = "scheduled"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// Scheduler 调度器接口
type Scheduler interface {
	SubmitTask(ctx context.Context, task *Task) error
	CancelTask(ctx context.Context, taskID string) error
	GetTask(ctx context.Context, taskID string) (*Task, error)
	ListTasks(ctx context.Context, filters map[string]string) ([]*Task, error)
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// SchedulingStrategy 调度策略：决定抢占关系，并可通过 AdjustWeights 调整打分插件权重
type SchedulingStrategy interface {
	ShouldPreempt(ctx context.Context, newTask *Task, runningTask *Task) bool
}

// BinpackStrategy Binpack 调度策略（默认）：优先填满节点
type BinpackStrategy struct{}

// ShouldPreempt 高优先级任务可以抢占低优先级任务
func (s *BinpackStrategy) ShouldPreempt(ctx context.Context, newTask *Task, runningTask *Task) bool {
	return newTask.Priority > runningTask.Priority
}

// SpreadStrategy Spread 调度策略：负载均衡，分散到不同节点
type SpreadStrategy struct{}

// ShouldPreempt 高优先级任务可以抢占低优先级任务
func (s *SpreadStrategy) ShouldPreempt(ctx context.Context, newTask *Task, runningTask *Task) bool {
	return newTask.Priority > runningTask.Priority
}

// AdjustWeights 关闭 BinPack、开启 LoadBalance
func (s *SpreadStrategy) AdjustWeights(w map[string]int) {
	delete(w, "BinPack")
	if w["LoadBalance"] == 0 {
		w["LoadBalance"] = 2
	}
}

// LifecycleEventType 生命周期事件类型
type LifecycleEventType string

const (
	LifecycleStarted     LifecycleEventType = "started"
	LifecycleFinished    LifecycleEventType = "finished"
	LifecyclePreempted   LifecycleEventType = "preempted"
	LifecycleRescheduled LifecycleEventType = "rescheduled"
	LifecycleMigrated    LifecycleEventType = "migrated"
	LifecycleScaled      LifecycleEventType = "scaled"
)

// LifecycleEvent 任务生命周期事件（在调度器锁外分发）
type LifecycleEvent struct {
	Type    LifecycleEventType `json:"type"`
	Task    Task               `json:"task"`
	Message string             `json:"message,omitempty"`
}

// Stats 调度统计
type Stats struct {
	Scheduled         int64   `json:"scheduled"`
	Unschedulable     int64   `json:"unschedulable_attempts"`
	Preemptions       int64   `json:"preemptions"`
	Reschedules       int64   `json:"reschedules"`
	Migrations        int64   `json:"migrations"`
	ElasticScaleUps   int64   `json:"elastic_scale_ups"`
	Completed         int64   `json:"completed"`
	Failed            int64   `json:"failed"`
	LastCycleMillis   float64 `json:"last_cycle_millis"`
	LastCycleAt       string  `json:"last_cycle_at,omitempty"`
	PendingTasks      int     `json:"pending_tasks"`
	RunningTasks      int     `json:"running_tasks"`
	AvgPendingSeconds float64 `json:"avg_pending_seconds"`
	MaxPendingSeconds float64 `json:"max_pending_seconds"`
}

// IntelligentScheduler 智能调度器
type IntelligentScheduler struct {
	resourcePool pool.ResourcePool
	inspector    NodeInspector
	queues       *QueueManager
	tasks        map[string]*Task
	runningTasks map[string]*Task
	cfg          Config
	stats        Stats
	hooks        []func(LifecycleEvent)
	pendingEvts  []LifecycleEvent
	mu           sync.RWMutex

	defaultStrategy   SchedulingStrategy
	inferenceStrategy SchedulingStrategy
	trainingStrategy  SchedulingStrategy

	now     func() time.Time
	trigger chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewIntelligentScheduler 创建智能调度器
func NewIntelligentScheduler(resourcePool pool.ResourcePool, strategy SchedulingStrategy) *IntelligentScheduler {
	ctx, cancel := context.WithCancel(context.Background())
	if strategy == nil {
		strategy = &BinpackStrategy{}
	}
	s := &IntelligentScheduler{
		resourcePool:      resourcePool,
		queues:            NewQueueManager(),
		tasks:             make(map[string]*Task),
		runningTasks:      make(map[string]*Task),
		cfg:               DefaultConfig(),
		defaultStrategy:   strategy,
		inferenceStrategy: &InferenceStrategy{BinpackStrategy: &BinpackStrategy{}},
		trainingStrategy:  &TrainingStrategy{BinpackStrategy: &BinpackStrategy{}},
		now:               time.Now,
		trigger:           make(chan struct{}, 1),
		ctx:               ctx,
		cancel:            cancel,
	}
	s.applyOvercommit()
	return s
}

// SetNodeInspector 注入节点检查器（集群管理器）
func (s *IntelligentScheduler) SetNodeInspector(ni NodeInspector) {
	s.mu.Lock()
	s.inspector = ni
	s.mu.Unlock()
}

// Queues 返回队列管理器
func (s *IntelligentScheduler) Queues() *QueueManager { return s.queues }

// AddLifecycleHook 注册生命周期回调；回调在调度器锁外执行
func (s *IntelligentScheduler) AddLifecycleHook(fn func(LifecycleEvent)) {
	s.mu.Lock()
	s.hooks = append(s.hooks, fn)
	s.mu.Unlock()
}

// GetConfig 获取调度配置
func (s *IntelligentScheduler) GetConfig() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// UpdateConfig 更新调度配置
func (s *IntelligentScheduler) UpdateConfig(cfg Config) error {
	if cfg.CoreOvercommit < 1 {
		return fmt.Errorf("core_overcommit must be >= 1")
	}
	if cfg.IntervalMillis <= 0 {
		cfg.IntervalMillis = 1000
	}
	for name := range cfg.Scores {
		if !knownScore(name) {
			return fmt.Errorf("unknown score plugin %q", name)
		}
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	s.applyOvercommit()
	s.Trigger()
	return nil
}

func knownScore(name string) bool {
	for _, p := range allScores {
		if p.Name() == name {
			return true
		}
	}
	return false
}

func (s *IntelligentScheduler) applyOvercommit() {
	if oc, ok := s.resourcePool.(interface{ SetCoreOvercommit(float64) }); ok {
		oc.SetCoreOvercommit(s.GetConfig().CoreOvercommit)
	}
}

// GetStats 获取调度统计
func (s *IntelligentScheduler) GetStats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := s.stats
	now := s.now()
	var sum float64
	for _, t := range s.tasks {
		switch t.Status {
		case TaskStatusPending:
			st.PendingTasks++
			w := now.Sub(t.CreatedAt).Seconds()
			sum += w
			if w > st.MaxPendingSeconds {
				st.MaxPendingSeconds = w
			}
		case TaskStatusRunning:
			st.RunningTasks++
		}
	}
	if st.PendingTasks > 0 {
		st.AvgPendingSeconds = sum / float64(st.PendingTasks)
	}
	return st
}

// Trigger 立即触发一次调度周期
func (s *IntelligentScheduler) Trigger() {
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

// SubmitTask 提交任务
func (s *IntelligentScheduler) SubmitTask(ctx context.Context, task *Task) error {
	if task == nil || task.ResourceReq == nil {
		return fmt.Errorf("resource_req is required")
	}
	if task.ResourceReq.Count <= 0 {
		task.ResourceReq.Count = 1
	}
	if task.ResourceReq.ResourceType == "" {
		task.ResourceReq.ResourceType = pool.ResourceTypeGPU
	}
	if task.MinCount > task.ResourceReq.Count {
		return fmt.Errorf("min_count %d > count %d", task.MinCount, task.ResourceReq.Count)
	}
	if task.Type == "" {
		task.Type = TaskType(task.ResourceReq.TaskType)
	}
	if task.Type == "" {
		task.Type = TaskTypeInference
	}
	if task.Priority == 0 {
		task.Priority = PriorityNormal
	}
	if task.TenantID == "" {
		task.TenantID = task.ResourceReq.TenantID
	}
	if task.Labels == nil {
		task.Labels = make(map[string]string)
	}
	if task.GangGroup == "" {
		task.GangGroup = task.Labels["gang_group"]
	}

	s.mu.Lock()
	if task.ID == "" {
		task.ID = fmt.Sprintf("task-%d", s.now().UnixNano())
	}
	if _, exists := s.tasks[task.ID]; exists {
		s.mu.Unlock()
		return fmt.Errorf("task already exists: %s", task.ID)
	}
	task.ResourceReq.TaskID = task.ID
	task.ResourceReq.TaskType = string(task.Type)
	task.Queue = s.queues.ResolveQueue(task)
	task.Status = TaskStatusPending
	task.CreatedAt = s.now()
	task.addEvent(task.CreatedAt, "Submitted", "queued in %s", task.Queue)
	s.tasks[task.ID] = task
	s.mu.Unlock()
	s.Trigger()
	return nil
}

// CancelTask 取消任务
func (s *IntelligentScheduler) CancelTask(ctx context.Context, taskID string) error {
	s.mu.Lock()
	task, ok := s.tasks[taskID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("task not found: %s", taskID)
	}
	if task.Status != TaskStatusPending && task.Status != TaskStatusRunning {
		s.mu.Unlock()
		return fmt.Errorf("task %s already %s", taskID, task.Status)
	}
	s.finishLocked(task, TaskStatusCancelled, "cancelled by user")
	s.mu.Unlock()
	s.dispatch()
	s.Trigger()
	return nil
}

// CompleteTask 由工作负载上报完成（success=false 表示失败）
func (s *IntelligentScheduler) CompleteTask(ctx context.Context, taskID string, success bool, message string) error {
	s.mu.Lock()
	task, ok := s.tasks[taskID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("task not found: %s", taskID)
	}
	if task.Status != TaskStatusRunning {
		s.mu.Unlock()
		return fmt.Errorf("task %s is %s, not running", taskID, task.Status)
	}
	status := TaskStatusCompleted
	if !success {
		status = TaskStatusFailed
	}
	if message == "" {
		message = "reported by workload"
	}
	s.finishLocked(task, status, message)
	s.mu.Unlock()
	s.dispatch()
	s.Trigger()
	return nil
}

// GetTask 获取任务信息
func (s *IntelligentScheduler) GetTask(ctx context.Context, taskID string) (*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task, ok := s.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	return task, nil
}

// ListTasks 列出任务（按创建时间倒序）
func (s *IntelligentScheduler) ListTasks(ctx context.Context, filters map[string]string) ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var tasks []*Task
	for _, task := range s.tasks {
		if matchTaskFilters(task, filters) {
			tasks = append(tasks, task)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		if !tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
			return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
		}
		return tasks[i].ID < tasks[j].ID
	})
	return tasks, nil
}

// UsageByTenant 按租户统计活跃任务资源占用（运行中任务的实际占用 + 活跃任务数）
func (s *IntelligentScheduler) UsageByTenant() map[string]TenantUsage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]TenantUsage)
	for _, t := range s.tasks {
		if t.TenantID == "" || (t.Status != TaskStatusRunning && t.Status != TaskStatusPending) {
			continue
		}
		u := out[t.TenantID]
		u.Tasks++
		if t.Status == TaskStatusRunning {
			r := taskRequest(t, len(t.Slices))
			u.GPUs += r.GPU
			u.NPUs += r.NPU
			u.MemoryGB += r.MemoryGB
		}
		out[t.TenantID] = u
	}
	return out
}

// TenantUsage 租户用量
type TenantUsage struct {
	GPUs     int   `json:"gpus"`
	NPUs     int   `json:"npus"`
	MemoryGB int64 `json:"memory_gb"`
	Tasks    int   `json:"tasks"`
}

// Start 启动调度器
func (s *IntelligentScheduler) Start(ctx context.Context) error {
	s.wg.Add(1)
	go s.scheduleLoop()
	return nil
}

// Stop 停止调度器
func (s *IntelligentScheduler) Stop(ctx context.Context) error {
	s.cancel()
	s.wg.Wait()
	return nil
}

func (s *IntelligentScheduler) scheduleLoop() {
	defer s.wg.Done()
	for {
		interval := time.Duration(s.GetConfig().IntervalMillis) * time.Millisecond
		timer := time.NewTimer(interval)
		select {
		case <-s.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		case <-s.trigger:
			timer.Stop()
		}
		s.RunCycle()
	}
}

// RunCycle 执行一次完整调度周期
func (s *IntelligentScheduler) RunCycle() {
	s.mu.Lock()
	start := time.Now()
	s.scheduleLocked()
	s.stats.LastCycleMillis = float64(time.Since(start).Microseconds()) / 1000
	s.stats.LastCycleAt = s.now().Format(time.RFC3339)
	s.mu.Unlock()
	s.dispatch()
}

func (s *IntelligentScheduler) emit(typ LifecycleEventType, t *Task, msg string) {
	cp := *t
	cp.Events = nil
	s.pendingEvts = append(s.pendingEvts, LifecycleEvent{Type: typ, Task: cp, Message: msg})
}

// dispatch 在锁外分发生命周期事件
func (s *IntelligentScheduler) dispatch() {
	s.mu.Lock()
	evts := s.pendingEvts
	s.pendingEvts = nil
	hooks := make([]func(LifecycleEvent), len(s.hooks))
	copy(hooks, s.hooks)
	s.mu.Unlock()
	for _, e := range evts {
		for _, h := range hooks {
			h(e)
		}
	}
}

func (s *IntelligentScheduler) strategyFor(task *Task) SchedulingStrategy {
	if task.Type == TaskTypeTraining {
		return s.trainingStrategy
	}
	return s.inferenceStrategy
}

// weightsFor 合并全局配置、默认策略与任务类型策略的打分权重
func (s *IntelligentScheduler) weightsFor(task *Task) map[string]int {
	w := scoreWeights(s.cfg, s.defaultStrategy)
	if tuner, ok := s.strategyFor(task).(interface{ AdjustWeights(map[string]int) }); ok {
		tuner.AdjustWeights(w)
	}
	switch task.Labels[LabelPlacementPolicy] {
	case PolicySpread:
		(&SpreadStrategy{}).AdjustWeights(w)
	case PolicyBinpack:
		delete(w, "LoadBalance")
		w["BinPack"] += 2
		w["Fragmentation"] += 2
	case PolicyTopology:
		w["Topology"] += 5
	}
	return w
}

func (s *IntelligentScheduler) buildSnapshot() *Snapshot {
	resources, _ := s.resourcePool.ListResources(s.ctx, nil)
	return newSnapshot(resources, s.inspector, s.cfg.CoreOvercommit)
}

func clusterCapacity(snap *Snapshot) ResourceList {
	var c ResourceList
	for _, n := range snap.Nodes {
		for _, d := range n.Devices {
			switch d.Type {
			case pool.ResourceTypeGPU:
				c.GPU++
			case pool.ResourceTypeNPU:
				c.NPU++
			}
			c.MemoryGB += d.TotalMemory
		}
	}
	return c
}

// sortPending QueueSort：优先级 > 队列加权主导份额（公平调度）> 提交时间
func (s *IntelligentScheduler) sortPending() []*Task {
	var pending []*Task
	for _, t := range s.tasks {
		if t.Status == TaskStatusPending {
			pending = append(pending, t)
		}
	}
	sort.SliceStable(pending, func(i, j int) bool {
		a, b := pending[i], pending[j]
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if sa, sb := s.queues.share(a.Queue), s.queues.share(b.Queue); sa != sb {
			return sa < sb
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.ID < b.ID
	})
	return pending
}

func (s *IntelligentScheduler) scheduleLocked() {
	now := s.now()
	s.completeExpiredLocked(now)

	snap := s.buildSnapshot()
	capacity := clusterCapacity(snap)
	s.queues.recompute(s.tasks, capacity)

	blocked := make(map[pool.ResourceType]string)
	doneGangs := make(map[string]bool)
	for _, task := range s.sortPending() {
		if task.Status != TaskStatusPending {
			continue
		}
		rtype := task.ResourceReq.ResourceType
		if holder, ok := blocked[rtype]; ok && holder != task.ID {
			task.Message = fmt.Sprintf("waiting: %s resources reserved for starving task %s", rtype, holder)
			continue
		}

		var placed bool
		if task.GangGroup != "" && s.cfg.EnableGang {
			if doneGangs[task.GangGroup] {
				continue
			}
			doneGangs[task.GangGroup] = true
			placed = s.tryGangLocked(task.GangGroup, snap, now)
		} else {
			placed = s.tryScheduleLocked(task, snap, now)
			if !placed && s.cfg.EnablePreemption && s.tryPreemptLocked(task, snap, now) {
				snap = s.buildSnapshot()
				s.queues.recompute(s.tasks, capacity)
				placed = s.tryScheduleLocked(task, snap, now)
			}
		}
		if !placed {
			s.stats.Unschedulable++
			starving := s.cfg.StarvationSeconds > 0 && now.Sub(task.CreatedAt) > time.Duration(s.cfg.StarvationSeconds)*time.Second
			if !s.cfg.EnableBackfill || starving {
				blocked[rtype] = task.ID
			}
		}
	}

	if s.cfg.EnableElastic {
		s.elasticScaleUpLocked(snap, blocked, now)
	}
	s.queues.recompute(s.tasks, capacity)
}

func (s *IntelligentScheduler) completeExpiredLocked(now time.Time) {
	for _, t := range s.runningTasks {
		if t.ExpectedDurationSeconds > 0 && t.StartedAt != nil &&
			now.Sub(*t.StartedAt) >= time.Duration(t.ExpectedDurationSeconds)*time.Second {
			s.finishLocked(t, TaskStatusCompleted, "reached expected duration")
		}
	}
}

// finishLocked 结束任务并释放资源
func (s *IntelligentScheduler) finishLocked(t *Task, status TaskStatus, reason string) {
	s.releaseLocked(t)
	now := s.now()
	t.Status = status
	t.CompletedAt = &now
	t.Message = reason
	switch status {
	case TaskStatusCompleted:
		s.stats.Completed++
		t.addEvent(now, "Completed", "%s", reason)
	case TaskStatusFailed:
		s.stats.Failed++
		t.addEvent(now, "Failed", "%s", reason)
	default:
		t.addEvent(now, "Cancelled", "%s", reason)
	}
	s.emit(LifecycleFinished, t, reason)
}

func (s *IntelligentScheduler) releaseLocked(t *Task) {
	if t.AllocationID != "" {
		_ = s.resourcePool.ReleaseResources(s.ctx, t.AllocationID)
	}
	t.AllocationID, t.PoolID, t.Slices, t.Nodes = "", "", nil, nil
	delete(s.runningTasks, t.ID)
}

// tryScheduleLocked 单任务调度：队列准入 -> 评估 -> 绑定；弹性任务按 [MinCount, Count] 降级
func (s *IntelligentScheduler) tryScheduleLocked(task *Task, snap *Snapshot, now time.Time) bool {
	counts := []int{task.ResourceReq.Count}
	if s.cfg.EnableElastic && task.MinCount > 0 {
		for c := task.ResourceReq.Count - 1; c >= task.MinCount; c-- {
			counts = append(counts, c)
		}
	}
	weights := s.weightsFor(task)
	var firstReason string
	for _, count := range counts {
		req := taskRequest(task, count)
		if err := s.queues.admit(task.Queue, req); err != nil {
			if firstReason == "" {
				firstReason = err.Error()
			}
			continue
		}
		feasible, evals := evaluate(s.cfg, weights, snap, nil, task, count, nil)
		if len(feasible) == 0 {
			if firstReason == "" {
				firstReason = summarizeFailures(evals)
			}
			continue
		}
		best := feasible[0]
		if err := s.bindLocked(task, best, now); err != nil {
			if firstReason == "" {
				firstReason = "bind failed: " + err.Error()
			}
			continue
		}
		snap.Assume(task.Slices, best.Mode)
		s.queues.assume(task.Queue, taskRequest(task, len(task.Slices)))
		return true
	}
	if task.Message != firstReason {
		task.addEvent(now, "FailedScheduling", "%s", firstReason)
	}
	task.Message = firstReason
	return false
}

// bindLocked 按落位结果向资源池精确分配
func (s *IntelligentScheduler) bindLocked(task *Task, p *Placement, now time.Time) error {
	req := *task.ResourceReq
	req.Count = len(p.Slices)
	req.ResourceIDs = p.ResourceIDs()
	req.ShareMode = p.Mode
	req.TenantID = task.TenantID
	result, err := s.resourcePool.AllocateResources(s.ctx, &req)
	if err != nil {
		return err
	}
	task.AllocationID = result.AllocationID
	task.PoolID = result.PoolID
	task.Slices = result.Slices
	task.Nodes = nodesOf(result.Slices)
	task.Status = TaskStatusRunning
	task.StartedAt = &now
	task.Message = ""
	detail := ""
	if len(p.ScoreDetail) > 0 {
		keys := make([]string, 0, len(p.ScoreDetail))
		for k := range p.ScoreDetail {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%d", k, p.ScoreDetail[k]))
		}
		detail = " (" + strings.Join(parts, ", ") + ")"
	}
	task.addEvent(now, "Scheduled", "%s x%d on %s [%s] score=%d%s",
		p.Mode, len(p.Slices), strings.Join(task.Nodes, ","), strings.Join(req.ResourceIDs, ","), p.Score, detail)
	if task.MinCount > 0 && len(p.Slices) < task.ResourceReq.Count {
		task.addEvent(now, "ElasticDegraded", "started with %d/%d devices", len(p.Slices), task.ResourceReq.Count)
	}
	s.runningTasks[task.ID] = task
	s.stats.Scheduled++
	s.emit(LifecycleStarted, task, "")
	return nil
}

func nodesOf(slices []pool.AllocationSlice) []string {
	set := make(map[string]bool)
	var out []string
	for _, s := range slices {
		if !set[s.NodeID] {
			set[s.NodeID] = true
			out = append(out, s.NodeID)
		}
	}
	sort.Strings(out)
	return out
}

// tryGangLocked Gang 调度：组内全部成员在快照上可落位后才一起绑定（Permit 全有或全无）
func (s *IntelligentScheduler) tryGangLocked(group string, snap *Snapshot, now time.Time) bool {
	var members []*Task
	minMember := 1
	for _, t := range s.tasks {
		if t.GangGroup == group && t.Status == TaskStatusPending {
			members = append(members, t)
			if t.MinMember > minMember {
				minMember = t.MinMember
			}
		}
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	setMsg := func(msg string) {
		for _, m := range members {
			if m.Message != msg {
				m.addEvent(now, "GangWaiting", "%s", msg)
			}
			m.Message = msg
		}
	}
	if len(members) < minMember {
		setMsg(fmt.Sprintf("gang %s waiting for members: %d/%d", group, len(members), minMember))
		return false
	}

	perQueue := make(map[string]ResourceList)
	for _, m := range members {
		perQueue[m.Queue] = perQueue[m.Queue].add(taskRequest(m, 0))
	}
	for q, req := range perQueue {
		if err := s.queues.admit(q, req); err != nil {
			setMsg("gang " + group + ": " + err.Error())
			return false
		}
	}

	work := snap.Clone()
	state := &CycleState{GangDomains: map[string]int{}, GangNodes: map[string]int{}}
	placements := make([]*Placement, len(members))
	for i, m := range members {
		feasible, evals := evaluate(s.cfg, s.weightsFor(m), work, state, m, m.ResourceReq.Count, nil)
		if len(feasible) == 0 {
			setMsg(fmt.Sprintf("gang %s: member %s unschedulable: %s", group, m.ID, summarizeFailures(evals)))
			return false
		}
		p := feasible[0]
		placements[i] = p
		work.Assume(p.Slices, p.Mode)
		state.GangNodes[p.Node]++
		if d := work.Nodes[p.Node].Labels[LabelRDMADomain]; d != "" {
			state.GangDomains[d]++
		}
	}

	for i, m := range members {
		if err := s.bindLocked(m, placements[i], now); err != nil {
			for j := 0; j < i; j++ {
				s.releaseLocked(members[j])
				members[j].Status = TaskStatusPending
				members[j].StartedAt = nil
			}
			setMsg(fmt.Sprintf("gang %s bind failed: %v", group, err))
			return false
		}
	}
	domains := make([]string, 0, len(state.GangDomains))
	for d := range state.GangDomains {
		domains = append(domains, d)
	}
	sort.Strings(domains)
	for i, m := range members {
		snap.Assume(m.Slices, placements[i].Mode)
		s.queues.assume(m.Queue, taskRequest(m, len(m.Slices)))
		m.addEvent(now, "GangScheduled", "gang %s (%d members) admitted together, rdma domains=%v", group, len(members), domains)
	}
	return true
}

type preemptOption struct {
	node    string
	victims []*Task
	maxPrio TaskPriority
}

// tryPreemptLocked PostFilter 抢占：在快照上模拟移除最少、最低优先级的受害者直到任务可落位
func (s *IntelligentScheduler) tryPreemptLocked(task *Task, snap *Snapshot, now time.Time) bool {
	strategy := s.strategyFor(task)
	req := taskRequest(task, 0)
	reclaim := s.queues.underGuarantee(task.Queue, req)
	if err := s.queues.admit(task.Queue, req); err != nil {
		return false
	}

	var candidates []*Task
	for _, v := range s.runningTasks {
		if v.NonPreemptible || v.GangGroup != "" || v.ResourceReq.ResourceType != task.ResourceReq.ResourceType {
			continue
		}
		byPriority := strategy.ShouldPreempt(s.ctx, task, v)
		byReclaim := reclaim && v.Queue != task.Queue && s.queues.overGuarantee(v.Queue)
		if byPriority || byReclaim {
			candidates = append(candidates, v)
		}
	}
	if len(candidates) == 0 {
		return false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority < candidates[j].Priority
		}
		return candidates[i].StartedAt.After(*candidates[j].StartedAt)
	})

	weights := s.weightsFor(task)
	var best *preemptOption
	for _, node := range snap.order {
		var onNode []*Task
		for _, v := range candidates {
			for _, n := range v.Nodes {
				if n == node {
					onNode = append(onNode, v)
					break
				}
			}
		}
		if len(onNode) == 0 {
			continue
		}
		work := snap.Clone()
		var chosen []*Task
		for _, v := range onNode {
			work.Forget(v.Slices)
			chosen = append(chosen, v)
			feasible, _ := evaluate(s.cfg, weights, work, nil, task, task.ResourceReq.Count, map[string]bool{node: true})
			if len(feasible) > 0 {
				opt := &preemptOption{node: node, victims: chosen}
				for _, c := range chosen {
					if c.Priority > opt.maxPrio {
						opt.maxPrio = c.Priority
					}
				}
				if best == nil || len(opt.victims) < len(best.victims) ||
					(len(opt.victims) == len(best.victims) && opt.maxPrio < best.maxPrio) {
					best = opt
				}
				break
			}
		}
	}
	if best == nil {
		return false
	}
	for _, v := range best.victims {
		s.releaseLocked(v)
		v.Status = TaskStatusPending
		v.StartedAt = nil
		v.PreemptionCount++
		v.Message = fmt.Sprintf("preempted by %s (priority %d) on %s", task.ID, task.Priority, best.node)
		v.addEvent(now, "Preempted", "%s", v.Message)
		s.stats.Preemptions++
		s.emit(LifecyclePreempted, v, v.Message)
	}
	task.addEvent(now, "Preempting", "evicted %d task(s) on %s", len(best.victims), best.node)
	return true
}

// elasticScaleUpLocked 弹性扩容：降级启动的任务在没有同类排队任务时扩容回期望卡数
func (s *IntelligentScheduler) elasticScaleUpLocked(snap *Snapshot, blocked map[pool.ResourceType]string, now time.Time) {
	waiting := make(map[pool.ResourceType]bool)
	for _, t := range s.tasks {
		if t.Status == TaskStatusPending {
			waiting[t.ResourceReq.ResourceType] = true
		}
	}
	ids := make([]string, 0, len(s.runningTasks))
	for id := range s.runningTasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		t := s.runningTasks[id]
		if t.MinCount <= 0 || len(t.Slices) >= t.ResourceReq.Count || waiting[t.ResourceReq.ResourceType] {
			continue
		}
		oldSlices := append([]pool.AllocationSlice(nil), t.Slices...)
		oldCount := len(oldSlices)
		work := snap.Clone()
		work.Forget(oldSlices)
		extra := taskRequest(t, t.ResourceReq.Count)
		cur := taskRequest(t, oldCount)
		delta := ResourceList{extra.GPU - cur.GPU, extra.NPU - cur.NPU, extra.MemoryGB - cur.MemoryGB}
		if err := s.queues.admit(t.Queue, delta); err != nil {
			continue
		}
		feasible, _ := evaluate(s.cfg, s.weightsFor(t), work, nil, t, t.ResourceReq.Count, nil)
		if len(feasible) == 0 {
			continue
		}
		oldIDs := make([]string, 0, oldCount)
		for _, sl := range oldSlices {
			oldIDs = append(oldIDs, sl.ResourceID)
		}
		startedAt := t.StartedAt
		s.releaseLocked(t)
		if err := s.bindLocked(t, feasible[0], now); err != nil {
			// 回滚到原落位
			req := *t.ResourceReq
			req.Count, req.ResourceIDs, req.TenantID = oldCount, oldIDs, t.TenantID
			if res, rerr := s.resourcePool.AllocateResources(s.ctx, &req); rerr == nil {
				t.AllocationID, t.PoolID, t.Slices, t.Nodes = res.AllocationID, res.PoolID, res.Slices, nodesOf(res.Slices)
				t.StartedAt = startedAt
				s.runningTasks[t.ID] = t
			} else {
				s.finishLocked(t, TaskStatusFailed, "elastic scale-up rollback failed: "+rerr.Error())
			}
			continue
		}
		t.StartedAt = startedAt
		snap.Forget(oldSlices)
		snap.Assume(t.Slices, feasible[0].Mode)
		msg := fmt.Sprintf("elastic scale up %d -> %d devices", oldCount, len(t.Slices))
		if t.Labels["checkpoint.auto_resume"] == "true" {
			msg += ", resume from latest checkpoint"
		}
		t.addEvent(now, "ElasticScaled", "%s", msg)
		s.stats.ElasticScaleUps++
		s.emit(LifecycleScaled, t, msg)
	}
}

// EvaluateNodes 对指定节点执行 Filter/Score（不修改状态），用于 K8s 调度扩展器与调试
func (s *IntelligentScheduler) EvaluateNodes(task *Task, nodeNames []string) []NodeEvaluation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap := s.buildSnapshot()
	var only map[string]bool
	if len(nodeNames) > 0 {
		only = make(map[string]bool, len(nodeNames))
		for _, n := range nodeNames {
			only[n] = true
		}
	}
	count := 1
	if task.ResourceReq != nil && task.ResourceReq.Count > 0 {
		count = task.ResourceReq.Count
	}
	_, evals := evaluate(s.cfg, s.weightsFor(task), snap, nil, task, count, only)
	seen := make(map[string]bool)
	for _, e := range evals {
		seen[e.Node] = true
	}
	for _, n := range nodeNames {
		if !seen[n] {
			evals = append(evals, NodeEvaluation{Node: n, Reason: "node has no managed " + string(task.ResourceReq.ResourceType) + " devices"})
		}
	}
	return evals
}

func matchTaskFilters(task *Task, filters map[string]string) bool {
	for k, v := range filters {
		if v == "" {
			continue
		}
		switch k {
		case "status":
			if string(task.Status) != v {
				return false
			}
		case "type":
			if string(task.Type) != v {
				return false
			}
		case "queue":
			if task.Queue != v {
				return false
			}
		case "tenant_id":
			if task.TenantID != v {
				return false
			}
		case "gang_group":
			if task.GangGroup != v {
				return false
			}
		case "node_id":
			found := false
			for _, n := range task.Nodes {
				if n == v {
					found = true
				}
			}
			if !found {
				return false
			}
		case "priority":
			if fmt.Sprintf("%d", task.Priority) != v {
				return false
			}
		default:
			if task.Labels[k] != v {
				return false
			}
		}
	}
	return true
}
