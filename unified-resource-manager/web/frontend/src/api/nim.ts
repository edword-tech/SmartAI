import client from './client'

export interface NIMConfig {
  base_url: string
  api_key_credential_id?: string
  enabled: boolean
  default_namespace?: string
  updated_at: string
}

export const nimApi = {
  getConfig: (): Promise<NIMConfig> => client.get('/nim/config'),
  updateConfig: (config: Partial<NIMConfig>): Promise<NIMConfig> =>
    client.put('/nim/config', config),
}
