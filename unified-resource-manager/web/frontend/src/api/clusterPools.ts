import client from './client'
import type { Resource } from './pools'

/** 集群大池规格 */
export interface ClusterPoolSpec {
  id: string
  name: string
  description?: string
  type: 'gpu' | 'npu'
  node_ids: string[]
  pool_ids?: string[]
  labels?: Record<string, string>
  created_at?: string
  updated_at?: string
}

/** 大池聚合信息（与 PoolInfo 结构一致） */
export interface ClusterPoolInfo {
  id: string
  name: string
  type: 'gpu' | 'npu'
  total_count: number
  available_count: number
  allocated_count: number
  total_memory: number
  used_memory: number
  nodes: string[]
  created_at?: string
  updated_at?: string
}

/** 子池规格 */
export interface SubPoolSpec {
  id: string
  name: string
  description?: string
  parent_cluster_pool_id?: string
  parent_pool_id?: string
  quota_max_count?: number
  quota_max_memory_gb?: number
  slice_config_id?: string
  labels?: Record<string, string>
  created_at?: string
  updated_at?: string
}

/** 子池用量 */
export interface SubPoolUsage {
  count: number
  memory_gb: number
  updated_at?: string
}

/** 分配请求（与 pools 一致） */
export interface AllocateRequest {
  task_id?: string
  task_type?: string
  resource_type: 'gpu' | 'npu'
  count: number
  memory_gb?: number
  cores?: number
  priority?: number
  timeout?: string
  labels?: Record<string, string>
}

/** 分配结果 */
export interface AllocateResult {
  allocation_id: string
  resources: Resource[]
  expires_at?: string
}

export const clusterPoolsApi = {
  // ----- 集群大池 -----
  listClusterPools: (): Promise<ClusterPoolSpec[]> => client.get('/cluster-pools'),
  createClusterPool: (spec: Partial<ClusterPoolSpec>): Promise<ClusterPoolSpec> =>
    client.post('/cluster-pools', spec),
  getClusterPool: (id: string): Promise<ClusterPoolSpec> => client.get(`/cluster-pools/${id}`),
  updateClusterPool: (id: string, spec: Partial<ClusterPoolSpec>): Promise<ClusterPoolSpec> =>
    client.put(`/cluster-pools/${id}`, { ...spec, id }),
  deleteClusterPool: (id: string): Promise<void> => client.delete(`/cluster-pools/${id}`),
  getClusterPoolInfo: (id: string): Promise<ClusterPoolInfo> =>
    client.get(`/cluster-pools/${id}/info`),
  listClusterPoolResources: (id: string, filters?: Record<string, string>): Promise<Resource[]> =>
    client.get(`/cluster-pools/${id}/resources`, { params: filters }),
  allocateFromClusterPool: (clusterPoolId: string, req: AllocateRequest): Promise<AllocateResult> =>
    client.post(`/cluster-pools/${clusterPoolId}/allocate`, req),

  // ----- 子池 -----
  listSubPools: (params?: { cluster_pool_id?: string; parent_pool_id?: string }): Promise<SubPoolSpec[]> =>
    client.get('/sub-pools', { params }),
  createSubPool: (spec: Partial<SubPoolSpec>): Promise<SubPoolSpec> =>
    client.post('/sub-pools', spec),
  getSubPool: (id: string): Promise<SubPoolSpec> => client.get(`/sub-pools/${id}`),
  deleteSubPool: (id: string): Promise<void> => client.delete(`/sub-pools/${id}`),
  getSubPoolUsage: (id: string): Promise<SubPoolUsage> => client.get(`/sub-pools/${id}/usage`),
  allocateFromSubPool: (subPoolId: string, req: AllocateRequest): Promise<AllocateResult> =>
    client.post(`/sub-pools/${subPoolId}/allocate`, req),
}
