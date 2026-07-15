import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import FileServiceView from '../FileServiceView.vue'

const { getFileStorageSettings, updateFileStorageSettings, testFileStorageSettings } = vi.hoisted(() => ({
  getFileStorageSettings: vi.fn(),
  updateFileStorageSettings: vi.fn(),
  testFileStorageSettings: vi.fn(),
}))

vi.mock('@/api/fileService', () => ({
  getFileStorageSettings,
  updateFileStorageSettings,
  testFileStorageSettings,
}))

const settings = {
  schema_version: 1,
  backend: 'local' as const,
  source: 'database' as const,
  public_base_url: 'https://api-key.cc',
  effective_public_base_url: 'https://api-key.cc',
  retention_hours: 24,
  daily_max_count: 100,
  daily_max_bytes: 2147483648,
  local_path: '/app/data/agent-assets',
  secret_access_key_configured: false,
  s3: {
    endpoint: '', region: 'auto', bucket: '', access_key_id: '', secret_access_key: '',
    prefix: 'model-assets/', force_path_style: false,
  },
  usage: { active_files: 3, active_bytes: 4096, local_files: 2, s3_files: 1, expiring_within_1_hour: 1 },
}

describe('admin FileServiceView', () => {
  beforeEach(() => {
    getFileStorageSettings.mockReset().mockResolvedValue(structuredClone(settings))
    updateFileStorageSettings.mockReset().mockImplementation(async (input) => ({
      ...structuredClone(settings), ...input, source: 'database', effective_public_base_url: 'https://api-key.cc',
    }))
    testFileStorageSettings.mockReset().mockResolvedValue({ ok: true, backend: 's3', message: 'connection successful' })
  })

  it('loads shared storage status as an independent model infrastructure page', async () => {
    const wrapper = mount(FileServiceView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
        },
      },
    })
    await flushPromises()

    expect(getFileStorageSettings).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('文件服务')
    expect(wrapper.text()).toContain('有效文件')
    expect(wrapper.text()).toContain('/app/data/agent-assets')
    expect(wrapper.text()).toContain('未来模型任务')
    expect(wrapper.text()).not.toContain('Yingzo')
  })

  it('tests and saves an S3 configuration without requiring a replacement secret', async () => {
    getFileStorageSettings.mockResolvedValue({ ...structuredClone(settings), secret_access_key_configured: true })
    const wrapper = mount(FileServiceView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
        },
      },
    })
    await flushPromises()

    await wrapper.get('button:nth-of-type(2)').trigger('click')
    await wrapper.get('#file-service-endpoint').setValue('http://minio:9000')
    await wrapper.get('#file-service-bucket').setValue('model-assets')
    await wrapper.get('#file-service-access-key').setValue('access')
    await wrapper.get('button[type="submit"]').trigger('submit')
    await flushPromises()

    expect(updateFileStorageSettings).toHaveBeenCalledWith(expect.objectContaining({
      backend: 's3',
      s3: expect.objectContaining({ bucket: 'model-assets', secret_access_key: '' }),
    }))
  })
})
