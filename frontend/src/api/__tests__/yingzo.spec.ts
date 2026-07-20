import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put },
}))

import { uploadYingzoReleaseArtifactTransport, type YingzoArtifactTransferInput } from '@/api/yingzo'

const MiB = 1024 * 1024

function transferInput(file: File): YingzoArtifactTransferInput {
  return {
    file,
    artifact_kind: 'host_package',
    target: 'openai',
    os: 'macos',
    arch: 'any',
    runtime_protocol: 1,
  }
}

function artifact(filename: string) {
  return { data: { id: 'artifact-1', package_filename: filename, size_bytes: 1 } }
}

describe('Yingzo resumable artifact transport', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('uploads a 20 MiB file as three bounded chunks without calling the batch endpoint', async () => {
    const file = new File([new Uint8Array(20 * MiB)], 'yingzo-openai-macos-0.3.0.tar.gz')
    post
      .mockResolvedValueOnce({ data: { upload_id: 'upload-1', offset: 0, chunk_size: 8 * MiB } })
      .mockResolvedValueOnce(artifact(file.name))
    put.mockImplementation(async (_url, body: Blob, config) => ({
      data: { upload_id: 'upload-1', offset: Number(config.headers['Upload-Offset']) + body.size },
    }))

    await uploadYingzoReleaseArtifactTransport('release-1', transferInput(file))

    expect(put).toHaveBeenCalledTimes(3)
    expect(put.mock.calls.map((call) => (call[1] as Blob).size)).toEqual([8 * MiB, 8 * MiB, 4 * MiB])
    expect(put.mock.calls.every((call) => (call[1] as Blob).size <= 8 * MiB)).toBe(true)
    expect(post.mock.calls[0][1]).toEqual(expect.objectContaining({
      package_filename: file.name,
      size_bytes: file.size,
      client_upload_id: expect.any(String),
    }))
    expect([...post.mock.calls, ...put.mock.calls, ...get.mock.calls].flat().join(' ')).not.toContain('/artifacts/batch')
  })

  it('recovers the server offset when a successful chunk response is lost', async () => {
    const file = new File([new Uint8Array(10 * MiB)], 'yingzo-openai-macos-0.3.0.tar.gz')
    post
      .mockResolvedValueOnce({ data: { upload_id: 'upload-2', offset: 0, chunk_size: 8 * MiB } })
      .mockResolvedValueOnce(artifact(file.name))
    put
      .mockRejectedValueOnce({ isAxiosError: true, code: 'ERR_NETWORK', request: {}, message: 'response lost' })
      .mockResolvedValueOnce({ data: { upload_id: 'upload-2', offset: 10 * MiB } })
    get.mockResolvedValueOnce({ data: { upload_id: 'upload-2', offset: 8 * MiB, chunk_size: 8 * MiB } })

    await uploadYingzoReleaseArtifactTransport('release-1', transferInput(file))

    expect(put).toHaveBeenCalledTimes(2)
    expect(put.mock.calls[0][2].headers['Upload-Offset']).toBe('0')
    expect(put.mock.calls[1][2].headers['Upload-Offset']).toBe(String(8 * MiB))
    expect(get).toHaveBeenCalledWith('/admin/yingzo/releases/release-1/artifact-uploads/upload-2')
  })

  it('retries the same chunk three times with backoff when status has not advanced', async () => {
    vi.useFakeTimers()
    const file = new File([new Uint8Array(1 * MiB)], 'yingzo-openai-macos-0.3.0.tar.gz')
    post
      .mockResolvedValueOnce({ data: { upload_id: 'upload-3', offset: 0, chunk_size: 8 * MiB } })
      .mockResolvedValueOnce(artifact(file.name))
    put
      .mockRejectedValueOnce({ isAxiosError: true, code: 'ETIMEDOUT', request: {} })
      .mockRejectedValueOnce({ isAxiosError: true, code: 'ETIMEDOUT', request: {} })
      .mockRejectedValueOnce({ isAxiosError: true, code: 'ETIMEDOUT', request: {} })
      .mockResolvedValueOnce({ data: { upload_id: 'upload-3', offset: file.size } })
    get.mockResolvedValue({ data: { upload_id: 'upload-3', offset: 0, chunk_size: 8 * MiB } })

    const transfer = uploadYingzoReleaseArtifactTransport('release-1', transferInput(file))
    await vi.runAllTimersAsync()
    await transfer

    expect(put).toHaveBeenCalledTimes(4)
    expect(put.mock.calls.every((call) => call[2].headers['Upload-Offset'] === '0')).toBe(true)
  })

  it('continues from server status after an offset conflict', async () => {
    const file = new File([new Uint8Array(10 * MiB)], 'yingzo-openai-macos-0.3.0.tar.gz')
    post
      .mockResolvedValueOnce({ data: { upload_id: 'upload-4', offset: 0, chunk_size: 8 * MiB } })
      .mockResolvedValueOnce(artifact(file.name))
    put
      .mockRejectedValueOnce({ status: 409, error: { code: 'upload_offset_conflict' } })
      .mockResolvedValueOnce({ data: { upload_id: 'upload-4', offset: 10 * MiB } })
    get.mockResolvedValueOnce({ data: { upload_id: 'upload-4', offset: 8 * MiB, chunk_size: 8 * MiB } })

    await uploadYingzoReleaseArtifactTransport('release-1', transferInput(file))

    expect(put).toHaveBeenCalledTimes(2)
    expect(put.mock.calls[1][2].headers['Upload-Offset']).toBe(String(8 * MiB))
  })

  it('retries completion with the same upload session when its response is lost', async () => {
    vi.useFakeTimers()
    const file = new File([new Uint8Array(1 * MiB)], 'yingzo-openai-macos-0.3.0.tar.gz')
    post
      .mockResolvedValueOnce({ data: { upload_id: 'upload-5', offset: 0, chunk_size: 8 * MiB } })
      .mockRejectedValueOnce({ isAxiosError: true, code: 'ECONNABORTED', request: {}, message: 'complete response lost' })
      .mockResolvedValueOnce(artifact(file.name))
    put.mockResolvedValueOnce({ data: { upload_id: 'upload-5', offset: file.size } })

    const transfer = uploadYingzoReleaseArtifactTransport('release-1', transferInput(file))
    await vi.runAllTimersAsync()
    await transfer

    expect(post.mock.calls.filter((call) => String(call[0]).endsWith('/complete'))).toHaveLength(2)
    expect(get).not.toHaveBeenCalled()
  })

  it('recovers a completed artifact from its upload tombstone when every complete response is lost', async () => {
    vi.useFakeTimers()
    const file = new File([new Uint8Array(1 * MiB)], 'yingzo-openai-macos-0.3.0.tar.gz')
    const committed = {
      id: 'artifact-committed', artifact_kind: 'host_package', target: 'openai', os: 'macos', arch: 'any',
      package_filename: file.name, size_bytes: file.size,
    }
    post.mockResolvedValueOnce({ data: { upload_id: 'upload-6', offset: 0, chunk_size: 8 * MiB } })
    for (let attempt = 0; attempt < 4; attempt += 1) post.mockRejectedValueOnce({ status: 503 })
    put.mockResolvedValueOnce({ data: { upload_id: 'upload-6', offset: file.size } })
    get.mockResolvedValue({ data: {
      upload_id: 'upload-6', offset: file.size, chunk_size: 8 * MiB, status: 'completed', artifact: committed,
    } })

    const transfer = uploadYingzoReleaseArtifactTransport('release-1', transferInput(file))
    await vi.runAllTimersAsync()
    await expect(transfer).resolves.toEqual(committed)

    expect(post.mock.calls.filter((call) => String(call[0]).endsWith('/complete'))).toHaveLength(4)
    expect(get).toHaveBeenCalledWith('/admin/yingzo/releases/release-1/artifact-uploads/upload-6')
  })

  it('scopes client upload ids to a release slot while preserving retry idempotency', async () => {
    const file = new File([new Uint8Array(16)], 'yingzo-openai-macos-0.3.0.tar.gz')
    post
      .mockResolvedValueOnce({ data: { upload_id: 'upload-a', offset: file.size, chunk_size: 8 * MiB } })
      .mockResolvedValueOnce(artifact(file.name))
      .mockResolvedValueOnce({ data: { upload_id: 'upload-a', offset: file.size, chunk_size: 8 * MiB } })
      .mockResolvedValueOnce(artifact(file.name))
      .mockResolvedValueOnce({ data: { upload_id: 'upload-b', offset: file.size, chunk_size: 8 * MiB } })
      .mockResolvedValueOnce(artifact(file.name))

    await uploadYingzoReleaseArtifactTransport('release-a', transferInput(file))
    await uploadYingzoReleaseArtifactTransport('release-a', transferInput(file))
    await uploadYingzoReleaseArtifactTransport('release-b', transferInput(file))

    const initBodies = post.mock.calls
      .filter((call) => !String(call[0]).endsWith('/complete'))
      .map((call) => call[1])
    expect(initBodies[0].client_upload_id).toBe(initBodies[1].client_upload_id)
    expect(initBodies[2].client_upload_id).not.toBe(initBodies[0].client_upload_id)
  })
})
