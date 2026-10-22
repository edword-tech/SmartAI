import client from './client'

export interface PoolInfo {
  id: string
  name: string
  type: 'gpu' | 'npu'
  total_count: number
  available_count: number
  allocated_count: number
  total_memory: number
  used_memory: number
  nodes: string[]
  /** purpose: training | inference | dev; gpu_model: A100 | H100 | H200 | RTX | Spark | 其他 */
  labels?: Record<string, string>
  created_at: string
  updated_at: string
}

export interface Resource {
  id: string
  type: 'gpu' | 'npu'
  state: 'available' | 'allocated' | 'maintenance' | 'error'
  node_id: string
  device_id: string
  total_memory: number
  used_memory: number
  total_cores: number
  used_cores: number
  labels?: Record<string, string>
  created_at: string
  updated_at: string
}

export interface ResourceMetrics {
  resource_id: string
  timestamp: string
  memory_used: number
  memory_total: number
  cores_used: number
  cores_total: number
  utilization: number
  temperature: number
  power_usage: number
}

export interface CreatePoolRequest {
  name: string
  type: 'gpu' | 'npu'
  nodes?: string[]
  quota?: { max_count?: number; max_memory_gb?: number }
  /** purpose: training | inference | dev; gpu_model: A100 | H100 | H200 | RTX | Spark | 其他 */
  labels?: Record<string, string>
}

export interface CreatePoolResponse {
  message?: string
  pool_id?: string
}

export const poolsApi = {
  // 创建资源池
  createPool: (req: CreatePoolRequest): Promise<CreatePoolResponse> => {
    return client.post('/pools', { ...req, nodes: req.nodes ?? [] })
  },

  // 列出所有资源池
  listPools: (): Promise<PoolInfo[]> => {
    return client.get('/pools')
  },

  // 获取资源池信息
  getPool: (poolId: string): Promise<PoolInfo> => {
    return client.get(`/pools/${poolId}`)
  },

  // 列出资源池中的资源
  listResources: (poolId: string, filters?: Record<string, string>): Promise<Resource[]> => {
    return client.get(`/pools/${poolId}/resources`, { params: filters })
  },

  // 获取资源信息
  getResource: (poolId: string, resourceId: string): Promise<Resource> => {
    return client.get(`/pools/${poolId}/resources/${resourceId}`)
  },

  // 获取资源指标
  getResourceMetrics: (poolId: string, resourceId: string, duration: string = '1h'): Promise<ResourceMetrics> => {
    return client.get(`/pools/${poolId}/resources/${resourceId}/metrics`, {
      params: { duration },
    })
  },

  // 分配资源
  allocateResources: (poolId: string, request: any): Promise<any> => {
    return client.post(`/pools/${poolId}/allocate`, request)
  },

  // 释放资源
  releaseResources: (poolId: string, allocationId: string): Promise<void> => {
    return client.post(`/pools/${poolId}/release/${allocationId}`)
  },

  // 列出池的已分配记录（感知已分配与剩余）
  listPoolAllocations: (poolId: string): Promise<AllocationRecord[]> => {
    return client.get(`/pools/${poolId}/allocations`)
  },

  // 列出池内槽位（按切分配置分割，用于 GPU 分割视图）
  listPoolSlots: (poolId: string, sliceConfigId?: string): Promise<PoolSlot[]> => {
    return client.get(`/pools/${poolId}/slots`, { params: sliceConfigId ? { slice_config_id: sliceConfigId } : undefined })
  },

  // 在池上部署大模型（分配资源并绑定）
  deployModelOnPool: (poolId: string, body: { model_id: string; resource_req?: any }): Promise<DeployResult> => {
    return client.post(`/pools/${poolId}/deploy`, body)
  },

  // 部署记录列表
  listDeployments: (poolId?: string): Promise<DeploymentRecord[]> => {
    return client.get('/deployments', { params: poolId ? { pool_id: poolId } : undefined })
  },
}

export interface AllocationRecord {
  allocation_id: string
  pool_id: string
  resource_ids: string[]
  task_id: string
  created_at: string
}

export interface PoolSlot {
  slot_id: string
  resource_id: string
  node_id: string
  memory_gb: number
  cores: number
  state: string
}

export interface DeploymentRecord {
  id: string
  model_id: string
  allocation_id: string
  pool_id: string
  status: string
  created_at: string
}

export interface DeployResult {
  deployment_id: string
  allocation_id: string
  model_id: string
  pool_id: string
  resources: Resource[]
  status: string
}
