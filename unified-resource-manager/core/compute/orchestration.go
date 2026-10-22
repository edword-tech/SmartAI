/*
 * Unified Resource Manager - Compute Orchestration (算力编排)
 *
 * 算力编排：智能编排和调度算力资源
 * 功能包括：
 * - 训练/推理任务下发 (Training/Inference Task Distribution)
 * - 算力均衡调度 (Compute Balanced Scheduling) -> 调度框架 LoadBalance 打分
 * - 算力最小碎片调度 (Compute Minimum Fragment Scheduling) -> BinPack + Fragmentation 打分
 * - 弹性调度优化 (Elastic Scheduling Optimization) -> 弹性 [MinCount, Count] 降级启动与自动扩容
 * - 调度优化 -> 碎片整理（make-before-break 迁移切分实例，腾出整卡）
 */

package compute

import (
	"context"
	"fmt"
	"sync"

	"flexai/unified-resource-manager/core/pool"
	"flexai/unified-resource-manager/core/scheduler"
)

// ComputeOrchestration 算力编排管理器
type ComputeOrchestration struct {
	scheduler    scheduler.Scheduler
	resourcePool *pool.UnifiedResourcePool

	balancedStrategy    *BalancedSchedulingStrategy
	minFragmentStrategy *MinFragmentSchedulingStrategy
	elasticStrategy     *ElasticSchedulingStrategy

	mu sync.RWMutex
}

// BalancedSchedulingStrategy 均衡调度策略
type BalancedSchedulingStrategy struct {
	Policy string `json:"policy"`
}

// MinFragmentSchedulingStrategy 最小碎片调度策略
type MinFragmentSchedulingStrategy struct {
	Policy string `json:"policy"`
	// FragmentThreshold 碎片率超过该阈值时建议执行碎片整理
	FragmentThreshold float64 `json:"fragment_threshold"`
}

// ElasticSchedulingStrategy 弹性调度策略
type ElasticSchedulingStrategy struct {
	ScaleUpThreshold   float64 `json:"scale_up_threshold"`   // 集群利用率高于此值时新任务以弹性方式降级启动
	ScaleDownThreshold float64 `json:"scale_down_threshold"` // 集群利用率低于此值时弹性任务自动扩容回期望规模
	// MinRatio 弹性任务最少卡数占期望卡数的比例
	MinRatio     float64 `json:"min_ratio"`
	MinResources int     `json:"min_resources"`
	MaxResources int     `json:"max_resources"`
}

// OptimizationResult 调度优化结果
type OptimizationResult struct {
	Fragmentation scheduler.FragmentationReport `json:"fragmentation"`
	Plan          *scheduler.DefragPlan         `json:"plan,omitempty"`
	Executed      bool                          `json:"executed"`
	Message       string                        `json:"message"`
}

// NewComputeOrchestration 创建算力编排管理器
func NewComputeOrchestration(sched scheduler.Scheduler, resourcePool *pool.UnifiedResourcePool) *ComputeOrchestration {
	return &ComputeOrchestration{
		scheduler:           sched,
		resourcePool:        resourcePool,
		balancedStrategy:    &BalancedSchedulingStrategy{Policy: scheduler.PolicySpread},
		minFragmentStrategy: &MinFragmentSchedulingStrategy{Policy: scheduler.PolicyBinpack, FragmentThreshold: 0.1},
		elasticStrategy: &ElasticSchedulingStrategy{
			ScaleUpThreshold:   0.8,
			ScaleDownThreshold: 0.3,
			MinRatio:           0.5,
			MinResources:       1,
			MaxResources:       100,
		},
	}
}

// DistributeTask 下发任务（训练/推理），按策略设置任务级放置策略后交给调度框架
func (co *ComputeOrchestration) DistributeTask(ctx context.Context, task *scheduler.Task, strategy string) error {
	if task == nil {
		return fmt.Errorf("task is required")
	}
	if task.Labels == nil {
		task.Labels = make(map[string]string)
	}
	switch strategy {
	case "balanced":
		task.Labels[scheduler.LabelPlacementPolicy] = scheduler.PolicySpread
	case "min_fragment":
		task.Labels[scheduler.LabelPlacementPolicy] = scheduler.PolicyBinpack
	case "topology":
		task.Labels[scheduler.LabelPlacementPolicy] = scheduler.PolicyTopology
	case "elastic":
		co.applyElastic(ctx, task)
	case "":
	default:
		return fmt.Errorf("unknown strategy %q", strategy)
	}
	task.Labels["orchestration_strategy"] = strategy
	return co.scheduler.SubmitTask(ctx, task)
}

func (co *ComputeOrchestration) applyElastic(ctx context.Context, task *scheduler.Task) {
	if task.ResourceReq == nil || task.MinCount > 0 {
		return
	}
	co.mu.RLock()
	es := *co.elasticStrategy
	co.mu.RUnlock()
	count := task.ResourceReq.Count
	if count > es.MaxResources {
		count = es.MaxResources
		task.ResourceReq.Count = count
	}
	min := int(float64(count) * es.MinRatio)
	if min < es.MinResources {
		min = es.MinResources
	}
	if min > count {
		min = count
	}
	task.MinCount = min
	if util, err := co.getCurrentUtilization(ctx); err == nil {
		task.Labels["current_utilization"] = fmt.Sprintf("%.2f", util)
		if util >= es.ScaleUpThreshold {
			task.Labels["scale_action"] = "start_degraded"
		}
	}
}

// getCurrentUtilization 按显存计算当前集群利用率
func (co *ComputeOrchestration) getCurrentUtilization(ctx context.Context) (float64, error) {
	resources, err := co.resourcePool.ListResources(ctx, nil)
	if err != nil {
		return 0, err
	}
	var used, total int64
	for _, r := range resources {
		used += r.UsedMemory
		total += r.TotalMemory
	}
	if total == 0 {
		return 0, nil
	}
	return float64(used) / float64(total), nil
}

type defragmenter interface {
	AnalyzeFragmentation() scheduler.FragmentationReport
	PlanDefragmentation() scheduler.DefragPlan
	ExecuteDefragmentation() (scheduler.DefragPlan, error)
}

// OptimizeScheduling 调度优化：碎片率超过阈值时执行碎片整理；execute=false 时仅返回计划
func (co *ComputeOrchestration) OptimizeScheduling(ctx context.Context) error {
	_, err := co.Optimize(ctx, true, false)
	return err
}

// Optimize 分析碎片并按需执行整理；force 为 true 时忽略阈值
func (co *ComputeOrchestration) Optimize(ctx context.Context, execute, force bool) (*OptimizationResult, error) {
	d, ok := co.scheduler.(defragmenter)
	if !ok {
		return nil, fmt.Errorf("scheduler does not support defragmentation")
	}
	res := &OptimizationResult{Fragmentation: d.AnalyzeFragmentation()}
	co.mu.RLock()
	threshold := co.minFragmentStrategy.FragmentThreshold
	co.mu.RUnlock()
	worst := 0.0
	for _, st := range res.Fragmentation.Stats {
		if st.FragmentationRatio > worst {
			worst = st.FragmentationRatio
		}
	}
	if !force && worst <= threshold {
		res.Message = fmt.Sprintf("fragmentation %.1f%% <= threshold %.1f%%, nothing to do", worst*100, threshold*100)
		return res, nil
	}
	if !execute {
		plan := d.PlanDefragmentation()
		res.Plan = &plan
		res.Message = fmt.Sprintf("plan: %d moves, frees %d devices", len(plan.Moves), len(plan.FreedDevices))
		return res, nil
	}
	plan, err := d.ExecuteDefragmentation()
	res.Plan, res.Executed = &plan, true
	res.Message = fmt.Sprintf("executed %d moves, freed %d devices", len(plan.Moves), len(plan.FreedDevices))
	return res, err
}

// GetBalancedStrategy 获取均衡调度策略
func (co *ComputeOrchestration) GetBalancedStrategy() *BalancedSchedulingStrategy {
	return co.balancedStrategy
}

// GetMinFragmentStrategy 获取最小碎片调度策略
func (co *ComputeOrchestration) GetMinFragmentStrategy() *MinFragmentSchedulingStrategy {
	return co.minFragmentStrategy
}

// GetElasticStrategy 获取弹性调度策略
func (co *ComputeOrchestration) GetElasticStrategy() *ElasticSchedulingStrategy {
	return co.elasticStrategy
}
