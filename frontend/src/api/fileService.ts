import { apiClient } from './client'

export type FileStorageBackend = 'local' | 's3'

export interface FileStorageS3Config {
  endpoint: string
  region: string
  bucket: string
  access_key_id: string
  secret_access_key: string
  prefix: string
  force_path_style: boolean
}

export interface FileStorageUsage {
  active_files: number
  active_bytes: number
  local_files: number
  s3_files: number
  expiring_within_1_hour: number
}

export interface FileStorageSettings {
  schema_version: number
  backend: FileStorageBackend
  source: 'database' | 'environment' | 'default'
  public_base_url: string
  effective_public_base_url: string
  retention_hours: number
  daily_max_count: number
  daily_max_bytes: number
  local_path: string
  secret_access_key_configured: boolean
  s3: FileStorageS3Config
  usage: FileStorageUsage
}

export type FileStorageUpdate = Omit<
  FileStorageSettings,
  'source' | 'effective_public_base_url' | 'local_path' | 'secret_access_key_configured' | 'usage'
>

export interface FileStorageTestResult {
  ok: boolean
  backend: FileStorageBackend
  message: string
}

export async function getFileStorageSettings(): Promise<FileStorageSettings> {
  const { data } = await apiClient.get<FileStorageSettings>('/admin/file-service/settings')
  return data
}

export async function updateFileStorageSettings(input: FileStorageUpdate): Promise<FileStorageSettings> {
  const { data } = await apiClient.put<FileStorageSettings>('/admin/file-service/settings', input)
  return data
}

export async function testFileStorageSettings(input: FileStorageUpdate): Promise<FileStorageTestResult> {
  const { data } = await apiClient.post<FileStorageTestResult>('/admin/file-service/test', input)
  return data
}
