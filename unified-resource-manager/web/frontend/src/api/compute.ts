import client from './client'

// ========== 算力感知 ==========

export interface ResourceState {
  resource_id: string
  node_id: string
  type: 'gpu' | 'npu'
  state: 'available' | 'allocated' | 'maintenance' | 'error'
  health: 'healthy' | 'warning' | 'critical' | 'unknown'
  utilization: number
  temperature?: number
  power_usage?: number
  memory_usage: number
  core_usage: number
  last_seen: string
  labels?: Record<string, string>
}

export interface ClusterSettings {
  cluster_id: string
  name: string
  auto_discovery: boolean
  health_check_interval: string
  resource_tags?: Record<string, string>
  created_at?: string
  updated_at?: string
}

export const computePerceptionApi = {
  listResourceStates: (filters?: Record<string, string>): Promise<ResourceState[]> => {
    return client.get('/compute/perception/states', { params: filters })
  },

  getResourceState: (resourceId: string): Promise<ResourceState> => {
    return client.get(`/compute/perception/states/${resourceId}`)
  },

  bindResource: (resourceId: string, taskId: string): Promise<void> => {
    return client.post('/compute/perception/bind', { resource_id: resourceId, task_id: taskId })
  },

  unbindResource: (resourceId: string): Promise<void> => {
    return client.post('/compute/perception/unbind', { resource_id: resourceId })
  },

  getClusterSettings: (): Promise<ClusterSettings> => {
    return client.get('/compute/perception/cluster-settings')
  },

  updateClusterSettings: (settings: ClusterSettings): Promise<ClusterSettings> => {
    return client.put('/compute/perception/cluster-settings', settings)
  },
}

// ========== 算力质量 ==========

export interface SlicingConfig {
  id: string
  name: string
  resource_type: 'gpu' | 'npu'
  slice_size: {
    memory_gb: number
    cores: number
  }
  max_slices: number
  isolation: boolean
  labels?: Record<string, string>
  created_at?: string
  updated_at?: string
}

export interface ExclusiveConfig {
  id: string
  name: string
  resource_type: 'gpu' | 'npu'
  priority: number
  max_duration: string
  labels?: Record<string, string>
  created_at?: string
  updated_at?: string
}

export interface AllocationStrategy {
  id: string
  name: string
  strategy: 'binpack' | 'spread' | 'fair'
  weight?: Record<string, number>
  min_quality: number
  labels?: Record<string, string>
  created_at?: string
  updated_at?: string
}

export const computeQualityApi = {
  listSlicingConfigs: (): Promise<SlicingConfig[]> => {
    return client.get('/compute/quality/slicing-configs')
  },

  createSlicingConfig: (config: Omit<SlicingConfig, 'created_at' | 'updated_at'>): Promise<SlicingConfig> => {
    return client.post('/compute/quality/slicing-configs', config)
  },

  listExclusiveConfigs: (): Promise<ExclusiveConfig[]> => {
    return client.get('/compute/quality/exclusive-configs')
  },

  createExclusiveConfig: (config: Omit<ExclusiveConfig, 'created_at' | 'updated_at'>): Promise<ExclusiveConfig> => {
    return client.post('/compute/quality/exclusive-configs', config)
  },

  listAllocationStrategies: (): Promise<AllocationStrategy[]> => {
    return client.get('/compute/quality/allocation-strategies')
  },

  createAllocationStrategy: (strategy: Omit<AllocationStrategy, 'created_at' | 'updated_at'>): Promise<AllocationStrategy> => {
    return client.post('/compute/quality/allocation-strategies', strategy)
  },
}

// ========== 算力编排 ==========

export interface OrchestrationStrategies {
  balanced: {
    node_weights?: Record<string, number>
  }
  min_fragment: {
    fragment_threshold: number
  }
  elastic: {
    scale_up_threshold: number
    scale_down_threshold: number
    min_resources: number
    max_resources: number
  }
}

export const computeOrchestrationApi = {
  distributeTask: (task: any, strategy: 'balanced' | 'min_fragment' | 'elastic'): Promise<any> => {
    return client.post('/compute/orchestration/distribute', { task, strategy })
  },

  optimizeScheduling: (): Promise<void> => {
    return client.post('/compute/orchestration/optimize', {})
  },

  getStrategies: (): Promise<OrchestrationStrategies> => {
    return client.get('/compute/orchestration/strategies')
  },
}

// ========== 训练加速 ==========

export interface DataLoadingConfig {
  id: string
  task_id: string
  prefetch_size: number
  num_workers: number
  pin_memory: boolean
  async_loading: boolean
  cache_strategy: string
  labels?: Record<string, string>
  created_at?: string
  updated_at?: string
}

export interface DistributedTrainingConfig {
  id: string
  task_id: string
  strategy: 'ddp' | 'fsdp' | 'deepspeed'
  num_nodes: number
  num_gpus_per_node: number
  communication_backend: 'nccl' | 'gloo'
  mixed_precision: boolean
  gradient_checkpointing: boolean
  labels?: Record<string, string>
  created_at?: string
  updated_at?: string
}

export interface CheckpointConfig {
  id: string
  task_id: string
  interval: string | number  // 后端可能返回纳秒数
  max_checkpoints: number
  auto_resume: boolean
  fault_prediction: boolean
  storage_path: string
  labels?: Record<string, string>
  created_at?: string
  updated_at?: string
}

export interface MemoryOptimizationConfig {
  id: string
  task_id: string
  memory_efficient_attention: boolean
  gradient_accumulation: number
  activation_checkpointing: boolean
  offload_strategy: string
  max_memory_usage: number
  labels?: Record<string, string>
  created_at?: string
  updated_at?: string
}

export const trainingAccelerationApi = {
  listDataLoadingConfigs: (): Promise<DataLoadingConfig[]> => {
    return client.get('/compute/acceleration/data-loading')
  },

  listDistributedConfigs: (): Promise<DistributedTrainingConfig[]> => {
    return client.get('/compute/acceleration/distributed')
  },

  listCheckpointConfigs: (): Promise<CheckpointConfig[]> => {
    return client.get('/compute/acceleration/checkpoint')
  },

  listMemoryOptimizationConfigs: (): Promise<MemoryOptimizationConfig[]> => {
    return client.get('/compute/acceleration/memory')
  },

  configureDataLoading: (config: Omit<DataLoadingConfig, 'created_at' | 'updated_at'>): Promise<DataLoadingConfig> => {
    return client.post('/compute/acceleration/data-loading', config)
  },

  configureDistributedTraining: (config: Omit<DistributedTrainingConfig, 'created_at' | 'updated_at'>): Promise<DistributedTrainingConfig> => {
    return client.post('/compute/acceleration/distributed', config)
  },

  configureCheckpoint: (config: Omit<CheckpointConfig, 'created_at' | 'updated_at'>): Promise<CheckpointConfig> => {
    return client.post('/compute/acceleration/checkpoint', config)
  },

  configureMemoryOptimization: (config: Omit<MemoryOptimizationConfig, 'created_at' | 'updated_at'>): Promise<MemoryOptimizationConfig> => {
    return client.post('/compute/acceleration/memory', config)
  },

  optimizeTask: (taskId: string): Promise<void> => {
    return client.post(`/compute/acceleration/optimize/${taskId}`, {})
  },
}
