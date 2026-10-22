import client from './client'

export interface ResourceFlavor {
  id: string
  name: string
  type: 'gpu' | 'npu'
  model: string
  memory_gb: number
  cores: number
  for_usage: 'inference' | 'training' | 'mixed'
  labels?: Record<string, string>
}

export const flavorsApi = {
  listFlavors: (): Promise<ResourceFlavor[]> => {
    return client.get('/flavors')
  },
}
