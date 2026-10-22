/*
 * Unified Resource Manager - Training Task Scheduling Strategy
 *
 * 训练任务专用调度策略：拓扑优先（NVLink/NUMA/RDMA 亲和，降低通信开销），减少抢占
 */

package scheduler

import (
	"context"
)

// TrainingStrategy 训练任务调度策略
type TrainingStrategy struct {
	*BinpackStrategy
}

// AdjustWeights 训练任务提高拓扑权重，降低碎片权重（训练多为整卡）
func (s *TrainingStrategy) AdjustWeights(w map[string]int) {
	if _, ok := w["Topology"]; ok {
		w["Topology"] += 3
	}
	if v, ok := w["Fragmentation"]; ok && v > 1 {
		w["Fragmentation"] = 1
	}
}

// ShouldPreempt 训练任务抢占策略：训练任务通常不抢占其他任务
func (s *TrainingStrategy) ShouldPreempt(ctx context.Context, newTask *Task, runningTask *Task) bool {
	// 训练任务之间需要更高的优先级差
	if newTask.Type == TaskTypeTraining && runningTask.Type == TaskTypeTraining {
		return newTask.Priority > runningTask.Priority+2
	}
	// 训练任务不能抢占推理任务（保障在线服务 SLA）
	if newTask.Type == TaskTypeTraining && runningTask.Type == TaskTypeInference {
		return false
	}
	return newTask.Priority > runningTask.Priority
}
