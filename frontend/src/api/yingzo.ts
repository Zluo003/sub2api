import { apiClient } from './client'

export type YingzoHost = 'chatgpt-work' | 'codex' | 'claude-cowork' | 'claude-code'
export type YingzoHostFamily = 'openai' | 'claude' | 'combined'

export interface YingzoReleaseArtifact {
  id?: string
  release_id?: string
  host_family: YingzoHostFamily
  package_filename: string
  storage_backend?: 'local' | 's3'
  size_bytes: number
  sha256?: string
  created_at?: string
  updated_at?: string
}

export interface YingzoReleaseSummary {
  id?: string
  version: string
  status?: 'draft' | 'published' | 'superseded' | 'disabled'
  size_bytes: number
  artifacts: Partial<Record<'openai' | 'claude' | 'combined', YingzoReleaseArtifact>>
  signature?: string
  signature_status?: 'signed' | 'unsigned'
  min_codex_version: string
  min_claude_version: string
  release_notes?: string
  created_at?: string
  published_at?: string
  updated_at?: string
}

export interface YingzoInstallInstructions {
  host: YingzoHost
  host_family: 'openai' | 'claude'
  version: string
  signature?: string
  download_url: string
  expires_at: string
  prompt: string
}

export interface YingzoAdminSettings {
  public_origin: string
  effective_origin: string
  release_storage: string
}

export async function getLatestYingzoRelease(): Promise<YingzoReleaseSummary> {
  const { data } = await apiClient.get<YingzoReleaseSummary>('/agent/plugin/releases/latest')
  return data
}

export async function createYingzoInstallInstructions(host: YingzoHost): Promise<YingzoInstallInstructions> {
  const { data } = await apiClient.post<YingzoInstallInstructions>('/agent/plugin/install-instructions', { host })
  return data
}

export async function listYingzoReleases(): Promise<YingzoReleaseSummary[]> {
  const { data } = await apiClient.get<{ items: YingzoReleaseSummary[] }>('/admin/yingzo/releases')
  return data.items
}

export async function uploadYingzoRelease(form: FormData): Promise<YingzoReleaseSummary> {
  const { data } = await apiClient.post<YingzoReleaseSummary>('/admin/yingzo/releases', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
    timeout: 10 * 60 * 1000,
  })
  return data
}

export async function publishYingzoRelease(id: string): Promise<YingzoReleaseSummary> {
  const { data } = await apiClient.post<YingzoReleaseSummary>(`/admin/yingzo/releases/${encodeURIComponent(id)}/publish`)
  return data
}

export async function rollbackYingzoRelease(id: string): Promise<YingzoReleaseSummary> {
  const { data } = await apiClient.post<YingzoReleaseSummary>(`/admin/yingzo/releases/${encodeURIComponent(id)}/rollback`)
  return data
}

export async function disableYingzoRelease(id: string): Promise<void> {
  await apiClient.delete(`/admin/yingzo/releases/${encodeURIComponent(id)}`)
}

export async function getYingzoAdminSettings(): Promise<YingzoAdminSettings> {
  const { data } = await apiClient.get<YingzoAdminSettings>('/admin/yingzo/settings')
  return data
}

export async function updateYingzoAdminSettings(publicOrigin: string): Promise<YingzoAdminSettings> {
  const { data } = await apiClient.put<YingzoAdminSettings>('/admin/yingzo/settings', { public_origin: publicOrigin })
  return data
}
