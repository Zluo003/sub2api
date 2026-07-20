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

/**
 * A server-declared upload slot. Schema 3 uses this list so the UI does not
 * have to duplicate the release matrix when a package target changes.
 */
export interface YingzoArtifactRequirement {
  key?: string
  label?: string
  artifact_kind: YingzoArtifactKind
  target: Exclude<YingzoArtifactTarget, 'claude' | 'combined'>
  os: YingzoOperatingSystem
  arch: YingzoArchitecture
  package_filename?: string
  filename?: string
  format?: YingzoReleaseArtifact['format']
  content_type?: string
}

export interface YingzoReleaseSummary {
  id?: string
  version: string
  status?: 'draft' | 'published' | 'superseded' | 'disabled'
  channel?: YingzoReleaseChannel
  stable_eligible?: boolean
  distribution_schema_version?: 1 | 2 | 3
  runtime_protocol?: number
  compatibility?: Record<string, unknown>
  required_artifacts?: YingzoArtifactRequirement[]
  artifact_count?: number
  size_bytes?: number
  artifacts?: Partial<Record<string, YingzoReleaseArtifact>>
  artifact_matrix?: YingzoReleaseArtifact[]
  artifact_items?: YingzoReleaseArtifact[]
  signature?: string
  signature_status?: 'signed' | 'unsigned'
  native_signing?: {
    macos: { status: 'verified' | 'unverified' | 'failed' }
    windows: { status: 'verified' | 'unsigned' | 'failed' }
  }
  warning?: string
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
  signature_status?: 'unverified' | 'verified' | 'failed'
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
  stable_eligible?: boolean
  native_signing?: YingzoReleaseSummary['native_signing']
  warning?: string
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
  distribution_schema_version: 2 | 3
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

export interface YingzoArtifactTransferInput extends YingzoArtifactUploadInput {
  existing_artifact_id?: string
}

export interface YingzoArtifactTransferProgress {
  uploaded_bytes: number
  total_bytes: number
}

export interface YingzoArtifactTransferOptions {
  onProgress?: (progress: YingzoArtifactTransferProgress) => void
}

interface YingzoArtifactUploadSession {
  upload_id: string
  offset: number
  chunk_size: number
  total_bytes?: number
  artifact?: YingzoReleaseArtifact
}

interface YingzoArtifactChunkReceipt {
  upload_id: string
  offset: number
  accepted_bytes?: number
  replayed?: boolean
}

export interface YingzoBatchUploadDuplicate {
  filename: string
  reason: string
}

export interface YingzoBatchUploadResult {
  uploaded: YingzoReleaseArtifact[]
  skipped_duplicates: YingzoBatchUploadDuplicate[]
  ignored_files: string[]
  missing_artifacts: string[]
  complete: boolean
  expected_count: number
  received_count: number
  errors: Array<{ filename: string; message: string }>
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
  return uploadYingzoReleaseArtifactTransport(releaseID, input)
}

export async function replaceYingzoReleaseArtifact(releaseID: string, artifactID: string, input: YingzoArtifactUploadInput): Promise<YingzoReleaseArtifact> {
  return uploadYingzoReleaseArtifactTransport(releaseID, { ...input, existing_artifact_id: artifactID })
}

/**
 * The UI has exactly one artifact-transfer boundary. Directory uploads call
 * this helper once per server-declared slot; the transport implementation can
 * move from whole-file multipart to resumable chunks without changing the UI
 * orchestration or retry state.
 */
export async function uploadYingzoReleaseArtifactTransport(
  releaseID: string,
  input: YingzoArtifactTransferInput,
  options: YingzoArtifactTransferOptions = {},
): Promise<YingzoReleaseArtifact> {
  const baseURL = `/admin/yingzo/releases/${encodeURIComponent(releaseID)}/artifact-uploads`
  const { data: session } = await retryTransientRequest(() => apiClient.post<YingzoArtifactUploadSession>(baseURL, {
    artifact_kind: input.artifact_kind,
    target: input.target,
    os: input.os,
    arch: input.arch,
    runtime_protocol: input.runtime_protocol,
    package_filename: input.file.name,
    size_bytes: input.file.size,
    client_upload_id: clientUploadID(
      input.file,
      `${releaseID}:${input.target}:${input.os}:${input.arch}`,
    ),
  }))
  assertUploadSession(session, input.file.size)

  const uploadURL = `${baseURL}/${encodeURIComponent(session.upload_id)}`
  const chunkSize = Math.min(session.chunk_size, yingzoClientChunkMaxBytes)
  let offset = session.offset
  options.onProgress?.({ uploaded_bytes: offset, total_bytes: input.file.size })
  while (offset < input.file.size) {
    offset = await uploadArtifactChunkWithRecovery(uploadURL, input.file, offset, chunkSize, options)
    options.onProgress?.({ uploaded_bytes: offset, total_bytes: input.file.size })
  }

  try {
    const { data: artifact } = await retryTransientRequest(
      () => apiClient.post<YingzoReleaseArtifact>(`${uploadURL}/complete`, {}),
      yingzoTransferRetryCount,
    )
    return artifact
  } catch (error) {
    // A complete response can be lost after the artifact transaction commits.
    // The status endpoint may return that artifact, avoiding a false failure.
    const recovered = await readArtifactUploadStatus(uploadURL)
    if (recovered?.artifact) return recovered.artifact
    throw error
  }
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

/** Permanently remove a draft or an unpublished disabled release. */
export async function purgeYingzoRelease(id: string): Promise<void> {
  await apiClient.delete(`/admin/yingzo/releases/${encodeURIComponent(id)}/purge`)
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

const yingzoClientChunkMaxBytes = 8 * 1024 * 1024
const yingzoTransferRetryCount = 3
const yingzoTransferRetryBaseDelayMS = 150
const yingzoClientUploadIDs = new WeakMap<File, Map<string, string>>()

function clientUploadID(file: File, scope: string) {
  let scopedIDs = yingzoClientUploadIDs.get(file)
  const existing = scopedIDs?.get(scope)
  if (existing) return existing
  const generated = typeof globalThis.crypto?.randomUUID === 'function'
    ? globalThis.crypto.randomUUID()
    : `upload-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
  if (!scopedIDs) {
    scopedIDs = new Map<string, string>()
    yingzoClientUploadIDs.set(file, scopedIDs)
  }
  scopedIDs.set(scope, generated)
  return generated
}

function assertUploadSession(session: YingzoArtifactUploadSession, fileSize: number) {
  if (!session?.upload_id
    || !Number.isSafeInteger(session.offset)
    || session.offset < 0
    || session.offset > fileSize
    || !Number.isSafeInteger(session.chunk_size)
    || session.chunk_size <= 0) {
    throw new Error('Yingzo upload returned an invalid session')
  }
}

async function uploadArtifactChunkWithRecovery(
  uploadURL: string,
  file: File,
  initialOffset: number,
  chunkSize: number,
  options: YingzoArtifactTransferOptions,
) {
  const offset = initialOffset
  let retry = 0
  while (true) {
    const end = Math.min(offset + chunkSize, file.size)
    const chunk = file.slice(offset, end)
    try {
      const { data: receipt } = await apiClient.put<YingzoArtifactChunkReceipt>(uploadURL, chunk, {
        headers: { 'Content-Type': 'application/octet-stream', 'Upload-Offset': String(offset) },
        timeout: 10 * 60 * 1000,
        onUploadProgress: (event: { loaded: number }) => options.onProgress?.({
          uploaded_bytes: Math.min(offset + event.loaded, file.size),
          total_bytes: file.size,
        }),
      })
      if (!Number.isSafeInteger(receipt.offset) || receipt.offset <= offset || receipt.offset > file.size) {
        throw new Error('Yingzo upload returned an invalid chunk offset')
      }
      return receipt.offset
    } catch (error) {
      // Always ask for authoritative state. This handles a lost success
      // response and 409 offset conflicts without re-sending accepted bytes.
      const recovered = await recoverArtifactUploadOffset(uploadURL, file.size)
      if (recovered !== undefined && recovered !== offset) return recovered
      if (!isRetryableTransferError(error, true) || retry >= yingzoTransferRetryCount) throw error
      await transferRetryDelay(retry)
      retry += 1
    }
  }
}

async function recoverArtifactUploadOffset(uploadURL: string, fileSize: number) {
  const status = await readArtifactUploadStatus(uploadURL)
  if (status && Number.isSafeInteger(status.offset) && status.offset >= 0 && status.offset <= fileSize) return status.offset
  return undefined
}

async function readArtifactUploadStatus(uploadURL: string) {
  try {
    const { data } = await retryTransientRequest(
      () => apiClient.get<YingzoArtifactUploadSession>(uploadURL),
      1,
    )
    return data
  } catch {
    return undefined
  }
}

async function retryTransientRequest<T>(operation: () => Promise<T>, maxRetries = yingzoTransferRetryCount): Promise<T> {
  let retry = 0
  while (true) {
    try {
      return await operation()
    } catch (error) {
      if (!isRetryableTransferError(error) || retry >= maxRetries) throw error
      await transferRetryDelay(retry)
      retry += 1
    }
  }
}

function isRetryableTransferError(error: unknown, allowOffsetConflict = false) {
  const structured = error as {
    status?: unknown
    code?: unknown
    error?: { code?: unknown }
    isAxiosError?: boolean
    request?: unknown
    response?: unknown
  }
  const status = Number(structured?.status)
  const code = String(structured?.error?.code || structured?.code || '')
  const networkCodes = new Set(['ERR_NETWORK', 'ECONNABORTED', 'ETIMEDOUT', 'ECONNRESET', 'ENETUNREACH', 'EAI_AGAIN'])
  const noResponse = structured?.response === undefined && (structured?.isAxiosError === true || structured?.request !== undefined)
  return status === 0
    || status >= 500
    || networkCodes.has(code)
    || noResponse
    || (allowOffsetConflict && status === 409 && code === 'upload_offset_conflict')
}

function transferRetryDelay(retry: number) {
  return new Promise<void>((resolve) => {
    setTimeout(resolve, yingzoTransferRetryBaseDelayMS * (2 ** retry))
  })
}
