/*
 * Unified Resource Manager - Fault Rescheduling / Drain / Defragmentation
 *
 * - 故障重调度：设备/节点故障时释放受影响任务并重新排队（有重试上限，支持断点续训提示）
 * - 节点排空 (drain)：运维维护前迁移节点上所有任务
 * - 碎片分析与整理：将可迁移的切分实例 make-before-break 地合并到已占用卡上，腾出整卡
 */

package scheduler

import (
	"fmt"
	"sort"
	"strings"

	"flexai/unified-resource-manager/core/pool"
)

func touches(t *Task, resourceIDs map[string]bool, nodeID string) bool {
	for _, s := range t.Slices {
		if resourceIDs[s.ResourceID] || (nodeID != "" && s.NodeID == nodeID) {
			return true
		}
	}
	return false
}

// rescheduleLocked 释放任务并重新排队；countRetry 为 true 时计入重试次数
func (s *IntelligentScheduler) rescheduleLocked(t *Task, reason string, countRetry bool) {
	now := s.now()
	s.releaseLocked(t)
	if countRetry {
		t.RetryCount++
		if t.RetryCount > s.cfg.MaxRetries {
			s.finishLocked(t, TaskStatusFailed, fmt.Sprintf("%s; exceeded max retries (%d)", reason, s.cfg.MaxRetries))
			return
		}
	}
	t.Status = TaskStatusPending
	t.StartedAt = nil
	msg := reason
	if t.Labels["checkpoint.auto_resume"] == "true" {
		msg += "; will resume from latest checkpoint"
	}
	t.Message = msg
	t.addEvent(now, "Rescheduled", "%s", msg)
	s.stats.Reschedules++
	s.emit(LifecycleRescheduled, t, msg)
}

func (s *IntelligentScheduler) evictLocked(resourceIDs []string, nodeID, reason string, countRetry bool) []string {
	set := make(map[string]bool, len(resourceIDs))
	for _, id := range resourceIDs {
		set[id] = true
	}
	var affected []string
	for _, t := range s.runningTasks {
		if touches(t, set, nodeID) {
			affected = append(affected, t.ID)
		}
	}
	sort.Strings(affected)
	for _, id := range affected {
		s.rescheduleLocked(s.tasks[id], reason, countRetry)
	}
	return affected
}

// HandleResourceFailure 设备故障：重调度使用这些设备的任务，返回受影响任务 ID
func (s *IntelligentScheduler) HandleResourceFailure(resourceIDs []string, reason string) []string {
	s.mu.Lock()
	affected := s.evictLocked(resourceIDs, "", "device failure: "+reason, true)
	s.mu.Unlock()
	s.dispatch()
	s.Trigger()
	return affected
}

// HandleNodeFailure 节点故障：重调度该节点上的全部任务
func (s *IntelligentScheduler) HandleNodeFailure(nodeID, reason string) []string {
	s.mu.Lock()
	affected := s.evictLocked(nil, nodeID, fmt.Sprintf("node %s failure: %s", nodeID, reason), true)
	s.mu.Unlock()
	s.dispatch()
	s.Trigger()
	return affected
}

// DrainNode 维护排空：迁移节点上的任务（不计入重试次数）
func (s *IntelligentScheduler) DrainNode(nodeID string) []string {
	s.mu.Lock()
	affected := s.evictLocked(nil, nodeID, fmt.Sprintf("node %s drained for maintenance", nodeID), false)
	s.mu.Unlock()
	s.dispatch()
	s.Trigger()
	return affected
}

// FragmentationStat 某类资源的碎片统计
type FragmentationStat struct {
	ResourceType       pool.ResourceType `json:"resource_type"`
	TotalDevices       int               `json:"total_devices"`
	IdleDevices        int               `json:"idle_devices"`
	PartialDevices     int               `json:"partial_devices"`
	FullDevices        int               `json:"full_devices"`
	FreeMemoryGB       int64             `json:"free_memory_gb"`
	FragmentedMemoryGB int64             `json:"fragmented_memory_gb"`
	FragmentationRatio float64           `json:"fragmentation_ratio"` // 碎片显存 / 总空闲显存
}

// NodeFragmentation 节点碎片情况
type NodeFragmentation struct {
	Node           string `json:"node"`
	IdleDevices    int    `json:"idle_devices"`
	PartialDevices int    `json:"partial_devices"`
	FreeMemoryGB   int64  `json:"free_memory_gb"`
	// LargestIdleGroup 最大的完全空闲 NVLink 组（可承接多卡训练的连续块）
	LargestIdleGroup int `json:"largest_idle_group"`
}

// FragmentationReport 碎片分析报告
type FragmentationReport struct {
	Stats []FragmentationStat `json:"stats"`
	Nodes []NodeFragmentation `json:"nodes"`
}

// AnalyzeFragmentation 碎片分析
func (s *IntelligentScheduler) AnalyzeFragmentation() FragmentationReport {
	s.mu.RLock()
	snap := s.buildSnapshot()
	s.mu.RUnlock()
	return analyzeSnapshot(snap)
}

func analyzeSnapshot(snap *Snapshot) FragmentationReport {
	stats := make(map[pool.ResourceType]*FragmentationStat)
	var rep FragmentationReport
	for _, name := range snap.order {
		n := snap.Nodes[name]
		nf := NodeFragmentation{Node: name}
		idleByGroup := make(map[string]int)
		for _, d := range n.Devices {
			st, ok := stats[d.Type]
			if !ok {
				st = &FragmentationStat{ResourceType: d.Type}
				stats[d.Type] = st
			}
			st.TotalDevices++
			if d.Cordoned || d.Health == pool.DeviceHealthFailed {
				continue
			}
			free := d.TotalMemory - d.UsedMemory
			switch {
			case d.Holders == 0:
				st.IdleDevices++
				nf.IdleDevices++
				key := topoKey(d, "nvlink")
				if key == "" {
					key = d.ID
				}
				idleByGroup[key]++
			case d.ShareMode == pool.ShareModeExclusive || free <= 0:
				st.FullDevices++
				free = 0
			default:
				st.PartialDevices++
				nf.PartialDevices++
				st.FragmentedMemoryGB += free
			}
			st.FreeMemoryGB += free
			nf.FreeMemoryGB += free
		}
		for _, c := range idleByGroup {
			if c > nf.LargestIdleGroup {
				nf.LargestIdleGroup = c
			}
		}
		rep.Nodes = append(rep.Nodes, nf)
	}
	for _, t := range []pool.ResourceType{pool.ResourceTypeGPU, pool.ResourceTypeNPU} {
		if st, ok := stats[t]; ok {
			if st.FreeMemoryGB > 0 {
				st.FragmentationRatio = float64(st.FragmentedMemoryGB) / float64(st.FreeMemoryGB)
			}
			rep.Stats = append(rep.Stats, *st)
		}
	}
	return rep
}

// DefragMove 一次迁移
type DefragMove struct {
	TaskID string `json:"task_id"`
	From   string `json:"from"`
	To     string `json:"to"`
}

// DefragPlan 碎片整理计划
type DefragPlan struct {
	Moves        []DefragMove        `json:"moves"`
	FreedDevices []string            `json:"freed_devices"`
	Before       FragmentationReport `json:"before"`
	After        FragmentationReport `json:"after"`
}

func migratable(t *Task) bool {
	if t.Status != TaskStatusRunning || len(t.Slices) != 1 || t.GangGroup != "" {
		return false
	}
	if v, ok := t.Labels["migratable"]; ok {
		return v == "true"
	}
	return t.Type == TaskTypeInference
}

// PlanDefragmentation 生成碎片整理计划：从占用最少的切分卡开始，把其上所有可迁移实例
// 按 best-fit 挪到其他已占用卡上，若全部挪走则该卡成为空闲整卡
func (s *IntelligentScheduler) PlanDefragmentation() DefragPlan {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.planDefragLocked()
}

func (s *IntelligentScheduler) planDefragLocked() DefragPlan {
	snap := s.buildSnapshot()
	plan := DefragPlan{Before: analyzeSnapshot(snap)}

	holders := make(map[string][]*Task)
	blocked := make(map[string]bool)
	for _, t := range s.runningTasks {
		for _, sl := range t.Slices {
			holders[sl.ResourceID] = append(holders[sl.ResourceID], t)
			if !migratable(t) {
				blocked[sl.ResourceID] = true
			}
		}
	}
	var sources []*pool.Resource
	for _, name := range snap.order {
		for _, d := range snap.Nodes[name].Devices {
			if d.Holders > 0 && d.ShareMode != pool.ShareModeExclusive && !blocked[d.ID] {
				sources = append(sources, d)
			}
		}
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].UsedMemory != sources[j].UsedMemory {
			return sources[i].UsedMemory < sources[j].UsedMemory
		}
		return sources[i].ID < sources[j].ID
	})

	freed := make(map[string]bool)
	for _, src := range sources {
		work := snap.Clone()
		tasks := holders[src.ID]
		sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
		for _, t := range tasks {
			work.Forget(t.Slices)
		}
		var moves []DefragMove
		ok := true
		for _, t := range tasks {
			target, slice := s.bestTarget(work, t, src.ID, freed)
			if target == "" {
				ok = false
				break
			}
			work.Assume([]pool.AllocationSlice{slice}, work.byID[target].ShareMode)
			moves = append(moves, DefragMove{TaskID: t.ID, From: src.ID, To: target})
		}
		if !ok || len(moves) == 0 {
			continue
		}
		snap = work
		freed[src.ID] = true
		plan.Moves = append(plan.Moves, moves...)
		plan.FreedDevices = append(plan.FreedDevices, src.ID)
	}
	plan.After = analyzeSnapshot(snap)
	return plan
}

// bestTarget 为任务选择迁移目标：已部分占用、同类型、共享方式一致、剩余显存最少的设备
func (s *IntelligentScheduler) bestTarget(snap *Snapshot, t *Task, exclude string, freed map[string]bool) (string, pool.AllocationSlice) {
	req := *t.ResourceReq
	req.Count = 1
	req.ShareMode = pool.ResolveShareMode(t.ResourceReq, 0)
	if cur, ok := snap.byID[t.Slices[0].ResourceID]; ok {
		req.ShareMode = pool.ResolveShareMode(t.ResourceReq, cur.TotalCores)
	}
	var bestID string
	var bestSlice pool.AllocationSlice
	var bestFree int64 = -1
	for _, name := range snap.order {
		node := snap.Nodes[name]
		if !node.Schedulable {
			continue
		}
		if st := (TenantIsolationFilter{}).Filter(t, node); st != nil {
			continue
		}
		for _, d := range node.Devices {
			if d.ID == exclude || freed[d.ID] || d.Holders == 0 || d.Type != req.ResourceType {
				continue
			}
			sl, err := pool.DeviceFit(d, &req, snap.CoreOvercommit)
			if err != nil {
				continue
			}
			free := d.TotalMemory - d.UsedMemory - sl.MemoryGB
			if bestFree < 0 || free < bestFree || (free == bestFree && d.ID < bestID) {
				bestID, bestSlice, bestFree = d.ID, sl, free
			}
		}
	}
	return bestID, bestSlice
}

// ExecuteDefragmentation 执行碎片整理：先在目标卡上分配（make），再释放原分配（break），
// 保证推理实例迁移期间不断服
func (s *IntelligentScheduler) ExecuteDefragmentation() (DefragPlan, error) {
	s.mu.Lock()
	plan := s.planDefragLocked()
	now := s.now()
	var errs []string
	for _, mv := range plan.Moves {
		t, ok := s.tasks[mv.TaskID]
		if !ok || !migratable(t) || t.Slices[0].ResourceID != mv.From {
			errs = append(errs, fmt.Sprintf("%s: state changed", mv.TaskID))
			continue
		}
		req := *t.ResourceReq
		req.Count, req.ResourceIDs, req.TenantID = 1, []string{mv.To}, t.TenantID
		res, err := s.resourcePool.AllocateResources(s.ctx, &req)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", mv.TaskID, err))
			continue
		}
		_ = s.resourcePool.ReleaseResources(s.ctx, t.AllocationID)
		t.AllocationID, t.PoolID, t.Slices, t.Nodes = res.AllocationID, res.PoolID, res.Slices, nodesOf(res.Slices)
		msg := fmt.Sprintf("defrag migrated %s -> %s", mv.From, mv.To)
		t.addEvent(now, "Migrated", "%s", msg)
		s.stats.Migrations++
		s.emit(LifecycleMigrated, t, msg)
	}
	s.mu.Unlock()
	s.dispatch()
	s.Trigger()
	if len(errs) > 0 {
		return plan, fmt.Errorf("some moves failed: %s", strings.Join(errs, "; "))
	}
	return plan, nil
}
