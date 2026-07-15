import { apiClient } from './client'

export type YingzoHost = 'codex' | 'claude-code'

export interface YingzoReleaseSummary {
  id?: string
  version: string
  status?: 'draft' | 'published' | 'superseded' | 'disabled'
  package_filename?: string
  size_bytes: number
  sha256: string
  signature?: string
  checksum_status?: 'verified'
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
  version: string
  sha256: string
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
