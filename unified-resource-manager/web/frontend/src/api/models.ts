import client from './client'

export interface Model {
  id: string
  name: string
  version: string
  type: string
  framework: string
  format?: string
  resource_flavor_id: string
  path: string
  description: string
  context_length?: number
  size?: number
  family?: string
  languages?: string[]
  abilities?: string[]
  project_id?: string
  inference_backend?: string  // '' | 'gpu' | 'nvidia_nim'
  nim_model_id?: string
  labels?: Record<string, string>
  tenant_id?: string
  created_at?: string
  updated_at?: string
}

export interface CreateModelRequest {
  id?: string
  name: string
  version: string
  type: string
  framework: string
  format?: string
  resource_flavor_id: string
  path: string
  description?: string
  context_length?: number
  size?: number
  family?: string
  languages?: string[]
  abilities?: string[]
  project_id?: string
  inference_backend?: string  // 'gpu' | 'nvidia_nim'
  nim_model_id?: string
  labels?: Record<string, string>
}

export const modelsApi = {
  listModels: (): Promise<Model[]> => {
    return client.get('/models')
  },

  getModel: (id: string): Promise<Model> => {
    return client.get(`/models/${id}`)
  },

  createModel: (req: CreateModelRequest): Promise<Model> => {
    return client.post('/models', req)
  },

  deleteModel: (id: string): Promise<void> => {
    return client.delete(`/models/${id}`)
  },
}

