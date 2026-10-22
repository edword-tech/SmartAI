import client from './client'

export interface NodeResources {
  gpus?: GPUInfo[]
  npus?: NPUInfo[]
}

export interface DeviceTopology {
  numa_node?: number
  pcie_switch?: string
  nvlink_group?: string
  rdma_nic?: string
}

export interface GPUInfo {
  device_id: string
  model: string
  total_memory: number
  total_cores: number
  topology?: DeviceTopology
}

export interface NPUInfo {
  device_id: string
  model: string
  total_memory: number
  total_cores: number
  topology?: DeviceTopology
}

export interface Node {
  id: string
  name: string
  type: 'gpu' | 'npu' | 'hybrid'
  state: 'online' | 'offline' | 'maintenance' | 'error'
  address: string
  labels?: Record<string, string>
  resources?: NodeResources
  network?: { rdma_domain?: string; rdma_nics?: { name: string; rate_gbps: number; state: string }[] }
  cordoned?: boolean
  last_seen: string
  created_at: string
  updated_at: string
}

export const nodesApi = {
  listNodes: (filters?: Record<string, string>): Promise<Node[]> => {
    return client.get('/nodes', { params: filters })
  },

  getNode: (nodeId: string): Promise<Node> => {
    return client.get(`/nodes/${nodeId}`)
  },

  discoverNodeResources: (nodeId: string): Promise<Node> => {
    return client.post(`/nodes/${nodeId}/discover`, {})
  },

  cordon: (nodeId: string, cordoned: boolean): Promise<void> => {
    return client.post(`/nodes/${nodeId}/cordon`, { cordoned })
  },

  drain: (nodeId: string): Promise<{ migrated_tasks: string[] }> => {
    return client.post(`/nodes/${nodeId}/drain`, {})
  },
}

