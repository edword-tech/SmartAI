import client from './client'

export interface Task {
  id: string
  name: string
  type: 'inference' | 'training'
  priority: number
  resource_req: {
    task_id: string
    task_type: string
    resource_type: 'gpu' | 'npu'
    count: number
    memory_gb: number
    cores: number
    priority: number
    timeout: string
    share_mode?: string
    mig_profile?: string
    labels?: Record<string, string>
  }
  status: 'pending' | 'scheduled' | 'running' | 'completed' | 'failed' | 'cancelled'
  queue?: string
  gang_group?: string
  min_member?: number
  min_count?: number
  nodes?: string[]
  slices?: { resource_id: string; node_id: string; memory_gb: number; cores: number }[]
  message?: string
  created_at: string
  started_at?: string
  completed_at?: string
  allocation_id?: string
  inference_backend?: string
  labels?: Record<string, string>
}

export interface SubmitTaskRequest {
  name: string
  type: 'inference' | 'training'
  priority: number
  resource_req: {
    task_id: string
    task_type: string
    resource_type: 'gpu' | 'npu'
    count: number
    memory_gb: number
    cores: number
    priority: number
    timeout: string
    share_mode?: string
    mig_profile?: string
    labels?: Record<string, string>
  }
  queue?: string
  gang_group?: string
  min_member?: number
  min_count?: number
  expected_duration_seconds?: number
  inference_backend?: string
  labels?: Record<string, string>
}

export const tasksApi = {
  // 列出所有任务
  listTasks: (filters?: Record<string, string>): Promise<Task[]> => {
    return client.get('/tasks', { params: filters })
  },

  // 获取任务信息
  getTask: (taskId: string): Promise<Task> => {
    return client.get(`/tasks/${taskId}`)
  },

  // 提交任务
  submitTask: (request: SubmitTaskRequest): Promise<Task> => {
    return client.post('/tasks', request)
  },

  // 取消任务
  cancelTask: (taskId: string): Promise<void> => {
    return client.delete(`/tasks/${taskId}`)
  },
}
