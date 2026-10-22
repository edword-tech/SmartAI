import client from './client'

export interface Project {
  id: string
  name: string
  description?: string
  tenant_id?: string
  owner_id?: string
  labels?: Record<string, string>
  created_at?: string
  updated_at?: string
}

export interface CreateProjectRequest {
  id?: string
  name: string
  description?: string
  tenant_id?: string
  owner_id?: string
  labels?: Record<string, string>
}

export const projectsApi = {
  listProjects: (): Promise<Project[]> => {
    return client.get('/projects')
  },

  getProject: (id: string): Promise<Project> => {
    return client.get(`/projects/${id}`)
  },

  createProject: (req: CreateProjectRequest): Promise<Project> => {
    return client.post('/projects', req)
  },

  updateProject: (id: string, req: CreateProjectRequest): Promise<Project> => {
    return client.put(`/projects/${id}`, req)
  },

  deleteProject: (id: string): Promise<void> => {
    return client.delete(`/projects/${id}`)
  },
}
