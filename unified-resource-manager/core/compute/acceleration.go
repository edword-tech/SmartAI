/*
 * Unified Resource Manager - Training Acceleration (训练加速)
 *
 * 训练加速：优化训练和推理性能
 * 功能包括：
 * - 训练数据加载优化 (Training Data Loading Optimization)
 * - 分布式训练 (Distributed Training)
 * - 断点续训/故障预测 (Checkpoint/Restart Training / Fault Prediction)
 * - 显存细粒度使用 (Fine-grained Memory Usage)
 */

package compute

import (
	"context"
	"fmt"
	"sync"
	"time"

	"flexai/unified-resource-manager/core/pool"
	"flexai/unified-resource-manager/core/scheduler"
)

// TrainingAcceleration 训练加速管理器
type TrainingAcceleration struct {
	scheduler    scheduler.Scheduler
	resourcePool *pool.UnifiedResourcePool
	
	// 数据加载优化配置
	dataLoadingConfigs map[string]*DataLoadingConfig
	// 分布式训练配置
	distributedConfigs map[string]*DistributedTrainingConfig
	// 检查点配置
	checkpointConfigs map[string]*CheckpointConfig
	// 显存优化配置
	memoryOptimizationConfigs map[string]*MemoryOptimizationConfig
	
	mu sync.RWMutex
}

// DataLoadingConfig 数据加载优化配置
type DataLoadingConfig struct {
	ID              string            `json:"id"`
	TaskID          string            `json:"task_id"`
	PrefetchSize    int               `json:"prefetch_size"`     // 预取大小
	NumWorkers      int               `json:"num_workers"`       // 工作线程数
	PinMemory       bool              `json:"pin_memory"`        // 固定内存
	AsyncLoading    bool              `json:"async_loading"`     // 异步加载
	CacheStrategy   string            `json:"cache_strategy"`    // 缓存策略
	Labels          map[string]string `json:"labels"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// DistributedTrainingConfig 分布式训练配置
type DistributedTrainingConfig struct {
	ID              string            `json:"id"`
	TaskID          string            `json:"task_id"`
	Strategy        string            `json:"strategy"`         // ddp/fsdp/deepspeed
	NumNodes        int               `json:"num_nodes"`         // 节点数
	NumGPUsPerNode  int               `json:"num_gpus_per_node"` // 每节点GPU数
	CommunicationBackend string       `json:"communication_backend"` // nccl/gloo
	MixedPrecision  bool              `json:"mixed_precision"`   // 混合精度
	GradientCheckpointing bool        `json:"gradient_checkpointing"` // 梯度检查点
	Labels          map[string]string `json:"labels"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// CheckpointConfig 检查点配置
type CheckpointConfig struct {
	ID              string            `json:"id"`
	TaskID          string            `json:"task_id"`
	Interval        time.Duration     `json:"interval"`         // 检查点间隔
	MaxCheckpoints  int               `json:"max_checkpoints"`  // 最大检查点数
	AutoResume      bool              `json:"auto_resume"`      // 自动恢复
	FaultPrediction bool              `json:"fault_prediction"` // 故障预测
	StoragePath     string            `json:"storage_path"`     // 存储路径
	Labels          map[string]string `json:"labels"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// MemoryOptimizationConfig 显存优化配置
type MemoryOptimizationConfig struct {
	ID              string            `json:"id"`
	TaskID          string            `json:"task_id"`
	MemoryEfficientAttention bool    `json:"memory_efficient_attention"` // 内存高效注意力
	GradientAccumulation int         `json:"gradient_accumulation"`      // 梯度累积步数
	ActivationCheckpointing bool     `json:"activation_checkpointing"`    // 激活检查点
	OffloadStrategy string           `json:"offload_strategy"`            // 卸载策略
	MaxMemoryUsage  int64            `json:"max_memory_usage"`           // 最大显存使用（GB）
	Labels          map[string]string `json:"labels"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// NewTrainingAcceleration 创建训练加速管理器
func NewTrainingAcceleration(sched scheduler.Scheduler, resourcePool *pool.UnifiedResourcePool) *TrainingAcceleration {
	return &TrainingAcceleration{
		scheduler:                 sched,
		resourcePool:              resourcePool,
		dataLoadingConfigs:        make(map[string]*DataLoadingConfig),
		distributedConfigs:        make(map[string]*DistributedTrainingConfig),
		checkpointConfigs:         make(map[string]*CheckpointConfig),
		memoryOptimizationConfigs: make(map[string]*MemoryOptimizationConfig),
	}
}

// ConfigureDataLoading 配置数据加载优化
func (ta *TrainingAcceleration) ConfigureDataLoading(config *DataLoadingConfig) error {
	if config == nil || config.ID == "" {
		return fmt.Errorf("invalid data loading config")
	}
	
	ta.mu.Lock()
	defer ta.mu.Unlock()
	
	now := time.Now()
	if config.CreatedAt.IsZero() {
		config.CreatedAt = now
	}
	config.UpdatedAt = now
	
	ta.dataLoadingConfigs[config.ID] = config
	return nil
}

// GetDataLoadingConfig 获取数据加载配置
func (ta *TrainingAcceleration) GetDataLoadingConfig(configID string) (*DataLoadingConfig, error) {
	ta.mu.RLock()
	defer ta.mu.RUnlock()
	
	config, ok := ta.dataLoadingConfigs[configID]
	if !ok {
		return nil, fmt.Errorf("data loading config not found: %s", configID)
	}
	
	return config, nil
}

// ListDataLoadingConfigs 列出所有数据加载配置
func (ta *TrainingAcceleration) ListDataLoadingConfigs() []*DataLoadingConfig {
	ta.mu.RLock()
	defer ta.mu.RUnlock()
	out := make([]*DataLoadingConfig, 0, len(ta.dataLoadingConfigs))
	for _, c := range ta.dataLoadingConfigs {
		out = append(out, c)
	}
	return out
}

// ConfigureDistributedTraining 配置分布式训练
func (ta *TrainingAcceleration) ConfigureDistributedTraining(config *DistributedTrainingConfig) error {
	if config == nil || config.ID == "" {
		return fmt.Errorf("invalid distributed training config")
	}
	
	ta.mu.Lock()
	defer ta.mu.Unlock()
	
	now := time.Now()
	if config.CreatedAt.IsZero() {
		config.CreatedAt = now
	}
	config.UpdatedAt = now
	
	ta.distributedConfigs[config.ID] = config
	return nil
}

// GetDistributedTrainingConfig 获取分布式训练配置
func (ta *TrainingAcceleration) GetDistributedTrainingConfig(configID string) (*DistributedTrainingConfig, error) {
	ta.mu.RLock()
	defer ta.mu.RUnlock()
	
	config, ok := ta.distributedConfigs[configID]
	if !ok {
		return nil, fmt.Errorf("distributed training config not found: %s", configID)
	}
	
	return config, nil
}

// ListDistributedConfigs 列出所有分布式训练配置
func (ta *TrainingAcceleration) ListDistributedConfigs() []*DistributedTrainingConfig {
	ta.mu.RLock()
	defer ta.mu.RUnlock()
	out := make([]*DistributedTrainingConfig, 0, len(ta.distributedConfigs))
	for _, c := range ta.distributedConfigs {
		out = append(out, c)
	}
	return out
}

// ConfigureCheckpoint 配置检查点
func (ta *TrainingAcceleration) ConfigureCheckpoint(config *CheckpointConfig) error {
	if config == nil || config.ID == "" {
		return fmt.Errorf("invalid checkpoint config")
	}
	
	ta.mu.Lock()
	defer ta.mu.Unlock()
	
	now := time.Now()
	if config.CreatedAt.IsZero() {
		config.CreatedAt = now
	}
	config.UpdatedAt = now
	
	ta.checkpointConfigs[config.ID] = config
	return nil
}

// GetCheckpointConfig 获取检查点配置
func (ta *TrainingAcceleration) GetCheckpointConfig(configID string) (*CheckpointConfig, error) {
	ta.mu.RLock()
	defer ta.mu.RUnlock()
	
	config, ok := ta.checkpointConfigs[configID]
	if !ok {
		return nil, fmt.Errorf("checkpoint config not found: %s", configID)
	}
	
	return config, nil
}

// ListCheckpointConfigs 列出所有检查点配置
func (ta *TrainingAcceleration) ListCheckpointConfigs() []*CheckpointConfig {
	ta.mu.RLock()
	defer ta.mu.RUnlock()
	out := make([]*CheckpointConfig, 0, len(ta.checkpointConfigs))
	for _, c := range ta.checkpointConfigs {
		out = append(out, c)
	}
	return out
}

// ConfigureMemoryOptimization 配置显存优化
func (ta *TrainingAcceleration) ConfigureMemoryOptimization(config *MemoryOptimizationConfig) error {
	if config == nil || config.ID == "" {
		return fmt.Errorf("invalid memory optimization config")
	}
	
	ta.mu.Lock()
	defer ta.mu.Unlock()
	
	now := time.Now()
	if config.CreatedAt.IsZero() {
		config.CreatedAt = now
	}
	config.UpdatedAt = now
	
	ta.memoryOptimizationConfigs[config.ID] = config
	return nil
}

// GetMemoryOptimizationConfig 获取显存优化配置
func (ta *TrainingAcceleration) GetMemoryOptimizationConfig(configID string) (*MemoryOptimizationConfig, error) {
	ta.mu.RLock()
	defer ta.mu.RUnlock()
	
	config, ok := ta.memoryOptimizationConfigs[configID]
	if !ok {
		return nil, fmt.Errorf("memory optimization config not found: %s", configID)
	}
	
	return config, nil
}

// ListMemoryOptimizationConfigs 列出所有显存优化配置
func (ta *TrainingAcceleration) ListMemoryOptimizationConfigs() []*MemoryOptimizationConfig {
	ta.mu.RLock()
	defer ta.mu.RUnlock()
	out := make([]*MemoryOptimizationConfig, 0, len(ta.memoryOptimizationConfigs))
	for _, c := range ta.memoryOptimizationConfigs {
		out = append(out, c)
	}
	return out
}

// OptimizeTask 优化任务（应用所有加速配置）
func (ta *TrainingAcceleration) OptimizeTask(ctx context.Context, taskID string) error {
	task, err := ta.scheduler.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	
	// 应用数据加载优化
	if dataConfig, err := ta.findDataLoadingConfigByTask(taskID); err == nil {
		ta.applyDataLoadingOptimization(task, dataConfig)
	}
	
	// 应用分布式训练配置
	if distConfig, err := ta.findDistributedConfigByTask(taskID); err == nil {
		ta.applyDistributedTraining(task, distConfig)
	}
	
	// 应用检查点配置
	if checkpointConfig, err := ta.findCheckpointConfigByTask(taskID); err == nil {
		ta.applyCheckpoint(task, checkpointConfig)
	}
	
	// 应用显存优化
	if memoryConfig, err := ta.findMemoryOptimizationConfigByTask(taskID); err == nil {
		ta.applyMemoryOptimization(task, memoryConfig)
	}
	
	return nil
}

// findDataLoadingConfigByTask 根据任务ID查找数据加载配置
func (ta *TrainingAcceleration) findDataLoadingConfigByTask(taskID string) (*DataLoadingConfig, error) {
	ta.mu.RLock()
	defer ta.mu.RUnlock()
	
	for _, config := range ta.dataLoadingConfigs {
		if config.TaskID == taskID {
			return config, nil
		}
	}
	
	return nil, fmt.Errorf("data loading config not found for task: %s", taskID)
}

// findDistributedConfigByTask 根据任务ID查找分布式训练配置
func (ta *TrainingAcceleration) findDistributedConfigByTask(taskID string) (*DistributedTrainingConfig, error) {
	ta.mu.RLock()
	defer ta.mu.RUnlock()
	
	for _, config := range ta.distributedConfigs {
		if config.TaskID == taskID {
			return config, nil
		}
	}
	
	return nil, fmt.Errorf("distributed training config not found for task: %s", taskID)
}

// findCheckpointConfigByTask 根据任务ID查找检查点配置
func (ta *TrainingAcceleration) findCheckpointConfigByTask(taskID string) (*CheckpointConfig, error) {
	ta.mu.RLock()
	defer ta.mu.RUnlock()
	
	for _, config := range ta.checkpointConfigs {
		if config.TaskID == taskID {
			return config, nil
		}
	}
	
	return nil, fmt.Errorf("checkpoint config not found for task: %s", taskID)
}

// findMemoryOptimizationConfigByTask 根据任务ID查找显存优化配置
func (ta *TrainingAcceleration) findMemoryOptimizationConfigByTask(taskID string) (*MemoryOptimizationConfig, error) {
	ta.mu.RLock()
	defer ta.mu.RUnlock()
	
	for _, config := range ta.memoryOptimizationConfigs {
		if config.TaskID == taskID {
			return config, nil
		}
	}
	
	return nil, fmt.Errorf("memory optimization config not found for task: %s", taskID)
}

// applyDataLoadingOptimization 应用数据加载优化
func (ta *TrainingAcceleration) applyDataLoadingOptimization(task *scheduler.Task, config *DataLoadingConfig) {
	if task.Labels == nil {
		task.Labels = make(map[string]string)
	}
	
	task.Labels["data_loading.prefetch_size"] = fmt.Sprintf("%d", config.PrefetchSize)
	task.Labels["data_loading.num_workers"] = fmt.Sprintf("%d", config.NumWorkers)
	task.Labels["data_loading.pin_memory"] = fmt.Sprintf("%v", config.PinMemory)
	task.Labels["data_loading.async_loading"] = fmt.Sprintf("%v", config.AsyncLoading)
	task.Labels["data_loading.cache_strategy"] = config.CacheStrategy
}

// applyDistributedTraining 应用分布式训练
func (ta *TrainingAcceleration) applyDistributedTraining(task *scheduler.Task, config *DistributedTrainingConfig) {
	if task.Labels == nil {
		task.Labels = make(map[string]string)
	}
	
	task.Labels["distributed.strategy"] = config.Strategy
	task.Labels["distributed.num_nodes"] = fmt.Sprintf("%d", config.NumNodes)
	task.Labels["distributed.num_gpus_per_node"] = fmt.Sprintf("%d", config.NumGPUsPerNode)
	task.Labels["distributed.communication_backend"] = config.CommunicationBackend
	task.Labels["distributed.mixed_precision"] = fmt.Sprintf("%v", config.MixedPrecision)
	task.Labels["distributed.gradient_checkpointing"] = fmt.Sprintf("%v", config.GradientCheckpointing)
}

// applyCheckpoint 应用检查点
func (ta *TrainingAcceleration) applyCheckpoint(task *scheduler.Task, config *CheckpointConfig) {
	if task.Labels == nil {
		task.Labels = make(map[string]string)
	}
	
	task.Labels["checkpoint.interval"] = config.Interval.String()
	task.Labels["checkpoint.max_checkpoints"] = fmt.Sprintf("%d", config.MaxCheckpoints)
	task.Labels["checkpoint.auto_resume"] = fmt.Sprintf("%v", config.AutoResume)
	task.Labels["checkpoint.fault_prediction"] = fmt.Sprintf("%v", config.FaultPrediction)
	task.Labels["checkpoint.storage_path"] = config.StoragePath
}

// applyMemoryOptimization 应用显存优化
func (ta *TrainingAcceleration) applyMemoryOptimization(task *scheduler.Task, config *MemoryOptimizationConfig) {
	if task.Labels == nil {
		task.Labels = make(map[string]string)
	}
	
	task.Labels["memory.memory_efficient_attention"] = fmt.Sprintf("%v", config.MemoryEfficientAttention)
	task.Labels["memory.gradient_accumulation"] = fmt.Sprintf("%d", config.GradientAccumulation)
	task.Labels["memory.activation_checkpointing"] = fmt.Sprintf("%v", config.ActivationCheckpointing)
	task.Labels["memory.offload_strategy"] = config.OffloadStrategy
	task.Labels["memory.max_memory_usage"] = fmt.Sprintf("%d", config.MaxMemoryUsage)
}
