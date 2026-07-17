import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import YingzoView from '../YingzoView.vue'

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  settings: vi.fn(),
  upload: vi.fn(),
  updateSettings: vi.fn(),
  publish: vi.fn(),
  rollback: vi.fn(),
  disable: vi.fn(),
}))

vi.mock('@/api/yingzo', () => ({
  listYingzoReleases: mocks.list,
  getYingzoAdminSettings: mocks.settings,
  uploadYingzoRelease: mocks.upload,
  updateYingzoAdminSettings: mocks.updateSettings,
  publishYingzoRelease: mocks.publish,
  rollbackYingzoRelease: mocks.rollback,
  disableYingzoRelease: mocks.disable,
}))

describe('Yingzo release administration', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.list.mockResolvedValue([])
    mocks.settings.mockResolvedValue({ public_origin: 'https://api-key.cc', effective_origin: 'https://api-key.cc', release_storage: '/data/releases' })
    mocks.upload.mockResolvedValue({ version: '0.2.0', artifacts: {} })
  })

  it('requires and submits one OpenAI package and one Claude package for the same version', async () => {
    const wrapper = mount(YingzoView, {
      global: {
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          RouterLink: { template: '<a><slot /></a>' },
          Icon: true,
        },
      },
    })
    await flushPromises()

    const files = wrapper.findAll('input[type="file"]')
    expect(files).toHaveLength(2)
    const openAI = new File(['openai'], 'yingzo-openai-0.2.0.tar.gz', { type: 'application/gzip' })
    const claude = new File(['claude'], 'yingzo-claude-0.2.0.tar.gz', { type: 'application/gzip' })
    Object.defineProperty(files[0].element, 'files', { configurable: true, value: [openAI] })
    Object.defineProperty(files[1].element, 'files', { configurable: true, value: [claude] })
    await files[0].trigger('change')
    await files[1].trigger('change')
    await wrapper.find('input[placeholder="0.1.3"]').setValue('0.2.0')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(mocks.upload).toHaveBeenCalledTimes(1)
    const form = mocks.upload.mock.calls[0][0] as FormData
    expect((form.get('openai_package') as File).name).toBe('yingzo-openai-0.2.0.tar.gz')
    expect((form.get('claude_package') as File).name).toBe('yingzo-claude-0.2.0.tar.gz')
    expect(form.get('version')).toBe('0.2.0')
  })
})
