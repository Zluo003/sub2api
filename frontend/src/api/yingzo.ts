import { apiClient } from './client'

export type YingzoHost = 'chatgpt-work' | 'codex' | 'claude-cowork' | 'claude-chat' | 'claude-code'
export type YingzoHostFamily = 'openai' | 'claude' | 'combined'
export type YingzoReleaseChannel = 'stable' | 'prerelease'
export type YingzoOperatingSystem = 'macos' | 'windows' | 'any'
export type YingzoArchitecture = 'arm64' | 'x64' | 'any'
export type YingzoArtifactKind = 'host_package' | 'runtime_installer'
export type YingzoArtifactTarget =
  | 'openai'
  | 'claude'
  | 'combined'
  | 'claude-code'
  | 'claude-desktop'
  | 'runtime'

export interface YingzoReleaseArtifact {
  id?: string
  release_id?: string
  host_family?: YingzoHostFamily
  artifact_kind?: YingzoArtifactKind
  target?: YingzoArtifactTarget
  os?: YingzoOperatingSystem
  arch?: YingzoArchitecture
  runtime_protocol?: number
  format?: 'tar.gz' | 'zip' | 'mcpb' | 'dmg' | 'exe'
  content_type?: string
  validation_status?: 'pending' | 'validated' | 'failed'
  signature_status?: 'unverified' | 'verified' | 'failed'
  validated_at?: string
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
  channel?: YingzoReleaseChannel
  distribution_schema_version?: 1 | 2
  runtime_protocol?: number
  compatibility?: Record<string, unknown>
  size_bytes?: number
  artifacts?: Partial<Record<string, YingzoReleaseArtifact>>
  artifact_matrix?: YingzoReleaseArtifact[]
  artifact_items?: YingzoReleaseArtifact[]
  signature?: string
  signature_status?: 'signed' | 'unsigned'
  min_codex_version: string
  min_claude_version: string
  release_notes?: string
  created_at?: string
  published_at?: string
  updated_at?: string
}

export interface YingzoArtifactDownload {
  artifact_id: string
  target: YingzoArtifactTarget
  os: YingzoOperatingSystem
  arch: YingzoArchitecture
  filename: string
  content_type: string
  download_url: string
  expires_at: string
}

export interface YingzoInstallRequest {
  host: YingzoHost
  os: Exclude<YingzoOperatingSystem, 'any'>
  arch: Exclude<YingzoArchitecture, 'any'>
  channel?: YingzoReleaseChannel
  installed_runtime_version?: string
  installed_runtime_protocol?: number
  runtime_capability?: 'unknown' | 'missing' | 'incompatible' | 'compatible'
}

export interface YingzoInstallInstructions {
  host: YingzoHost
  host_family: 'openai' | 'claude'
  version: string
  channel?: YingzoReleaseChannel
  runtime_protocol?: number
  signature?: string
  download_url: string
  expires_at: string
  prompt: string
  host_package?: YingzoArtifactDownload
  runtime_installer?: YingzoArtifactDownload | null
  runtime_installer_required?: boolean
  runtime_resolution?: 'probe' | 'required' | 'compatible'
  runtime_helper_uri?: string
}

export interface YingzoReleaseDraftInput {
  version: string
  channel: YingzoReleaseChannel
  distribution_schema_version: 2
  runtime_protocol: number
  compatibility: Record<string, unknown>
  min_codex_version?: string
  min_claude_version?: string
  release_notes?: string
}

export interface YingzoArtifactUploadInput {
  file: File
  artifact_kind: YingzoArtifactKind
  target: Extract<YingzoArtifactTarget, 'openai' | 'claude-code' | 'claude-desktop' | 'runtime'>
  os: YingzoOperatingSystem
  arch: YingzoArchitecture
  runtime_protocol: number
}

export interface YingzoReleaseProofInput {
  algorithm: 'Ed25519'
  key_id: string
  manifest_base64: string
  signature_base64: string
}

export interface YingzoAdminSettings {
  public_origin: string
  effective_origin: string
  release_storage: string
}

export async function getLatestYingzoRelease(channel: YingzoReleaseChannel = 'stable'): Promise<YingzoReleaseSummary> {
  const { data } = await apiClient.get<YingzoReleaseSummary>('/agent/plugin/releases/latest', { params: { channel } })
  return data
}

export async function createYingzoInstallInstructions(input: YingzoHost | YingzoInstallRequest): Promise<YingzoInstallInstructions> {
  const body: YingzoInstallRequest = typeof input === 'string'
    ? { host: input, os: 'macos', arch: 'arm64', channel: 'stable' }
    : input
  const { data } = await apiClient.post<YingzoInstallInstructions>('/agent/plugin/install-instructions', body)
  return data
}

export async function listYingzoReleases(): Promise<YingzoReleaseSummary[]> {
  const { data } = await apiClient.get<{ items: YingzoReleaseSummary[] }>('/admin/yingzo/releases')
  return data.items
}

export async function createYingzoReleaseDraft(input: YingzoReleaseDraftInput): Promise<YingzoReleaseSummary> {
  const { data } = await apiClient.post<YingzoReleaseSummary>('/admin/yingzo/releases', input)
  return data
}

export async function uploadYingzoReleaseArtifact(releaseID: string, input: YingzoArtifactUploadInput): Promise<YingzoReleaseArtifact> {
  const form = artifactForm(input)
  const { data } = await apiClient.post<YingzoReleaseArtifact>(`/admin/yingzo/releases/${encodeURIComponent(releaseID)}/artifacts`, form, uploadConfig)
  return data
}

export async function replaceYingzoReleaseArtifact(releaseID: string, artifactID: string, input: YingzoArtifactUploadInput): Promise<YingzoReleaseArtifact> {
  const form = artifactForm(input)
  const { data } = await apiClient.put<YingzoReleaseArtifact>(`/admin/yingzo/releases/${encodeURIComponent(releaseID)}/artifacts/${encodeURIComponent(artifactID)}`, form, uploadConfig)
  return data
}

export async function deleteYingzoReleaseArtifact(releaseID: string, artifactID: string): Promise<void> {
  await apiClient.delete(`/admin/yingzo/releases/${encodeURIComponent(releaseID)}/artifacts/${encodeURIComponent(artifactID)}`)
}

export async function verifyYingzoReleaseProof(releaseID: string, input: YingzoReleaseProofInput): Promise<{ verified: boolean; key_id: string; verified_at: string }> {
  const { data } = await apiClient.put(`/admin/yingzo/releases/${encodeURIComponent(releaseID)}/proof`, input)
  return data
}

// Kept for schema-1 automation while v0.2.x releases remain rollback-compatible.
export async function uploadYingzoRelease(form: FormData): Promise<YingzoReleaseSummary> {
  const { data } = await apiClient.post<YingzoReleaseSummary>('/admin/yingzo/releases', form, uploadConfig)
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

export async function promoteYingzoRelease(id: string): Promise<YingzoReleaseSummary> {
  const { data } = await apiClient.post<YingzoReleaseSummary>(`/admin/yingzo/releases/${encodeURIComponent(id)}/promote`)
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

const uploadConfig = {
  headers: { 'Content-Type': 'multipart/form-data' },
  timeout: 10 * 60 * 1000,
}

function artifactForm(input: YingzoArtifactUploadInput): FormData {
  const form = new FormData()
  form.set('file', input.file)
  form.set('artifact_kind', input.artifact_kind)
  form.set('target', input.target)
  form.set('os', input.os)
  form.set('arch', input.arch)
  form.set('runtime_protocol', String(input.runtime_protocol))
  return form
}
