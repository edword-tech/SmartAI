/*
 * Unified Resource Manager - Inference Task Scheduling Strategy
 *
 * 推理任务专用调度策略：小规格切分 + binpack 提高卡利用率，可抢占低优训练任务
 */

package scheduler

import (
	"context"
)

// InferenceStrategy 推理任务调度策略
type InferenceStrategy struct {
	*BinpackStrategy
}

// AdjustWeights 推理任务提高防碎片权重，把小规格实例紧凑装箱
func (s *InferenceStrategy) AdjustWeights(w map[string]int) {
	if _, ok := w["Fragmentation"]; ok {
		w["Fragmentation"]++
	}
}

// ShouldPreempt 推理任务抢占策略：推理任务优先级不低于训练任务时可以抢占训练任务
func (s *InferenceStrategy) ShouldPreempt(ctx context.Context, newTask *Task, runningTask *Task) bool {
	if newTask.Type == TaskTypeInference && runningTask.Type == TaskTypeTraining {
		return newTask.Priority >= runningTask.Priority
	}
	return newTask.Priority > runningTask.Priority
}
