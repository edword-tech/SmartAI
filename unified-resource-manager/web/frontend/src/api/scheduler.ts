import client from './client'

export interface SchedulerStats {
  scheduled: number
  unschedulable_attempts: number
  preemptions: number
  reschedules: number
  migrations: number
  elastic_scale_ups: number
  completed: number
  failed: number
  last_cycle_millis: number
  last_cycle_at?: string
  pending_tasks: number
  running_tasks: number
  avg_pending_seconds: number
  max_pending_seconds: number
}

export interface PluginConfig {
  enabled: boolean
  weight: number
}

export interface SchedulerConfig {
  filters: Record<string, boolean>
  scores: Record<string, PluginConfig>
  enable_preemption: boolean
  enable_gang: boolean
  enable_elastic: boolean
  enable_backfill: boolean
  starvation_seconds: number
  core_overcommit: number
  max_retries: number
  interval_millis: number
}

export interface ResourceList {
  gpu: number
  npu: number
  memory_gb: number
}

export interface Queue {
  name: string
  tenant_id?: string
  weight: number
  guaranteed: ResourceList
  capability: ResourceList
  reclaimable: boolean
  allocated: ResourceList
  pending: number
  running: number
  dominant_share: number
}

export interface FragmentationStat {
  resource_type: string
  total_devices: number
  idle_devices: number
  partial_devices: number
  full_devices: number
  free_memory_gb: number
  fragmented_memory_gb: number
  fragmentation_ratio: number
}

export interface NodeFragmentation {
  node: string
  idle_devices: number
  partial_devices: number
  free_memory_gb: number
  largest_idle_group: number
}

export interface FragmentationReport {
  stats: FragmentationStat[]
  nodes: NodeFragmentation[]
}

export interface DefragMove {
  task_id: string
  from: string
  to: string
}

export interface DefragPlan {
  moves: DefragMove[]
  freed_devices: string[]
  before: FragmentationReport
  after: FragmentationReport
}

export const schedulerApi = {
  getStats: (): Promise<SchedulerStats> => client.get('/scheduler/stats'),
  getConfig: (): Promise<SchedulerConfig> => client.get('/scheduler/config'),
  updateConfig: (cfg: SchedulerConfig): Promise<SchedulerConfig> => client.put('/scheduler/config', cfg),
  listQueues: (): Promise<Queue[]> => client.get('/scheduler/queues'),
  upsertQueue: (q: Partial<Queue> & { name: string }): Promise<Queue> => client.post('/scheduler/queues', q),
  deleteQueue: (name: string): Promise<void> => client.delete(`/scheduler/queues/${name}`),
  getFragmentation: (): Promise<FragmentationReport> => client.get('/scheduler/fragmentation'),
  planDefrag: (): Promise<DefragPlan> => client.get('/scheduler/defrag'),
  executeDefrag: (): Promise<DefragPlan> => client.post('/scheduler/defrag', {}),
}
