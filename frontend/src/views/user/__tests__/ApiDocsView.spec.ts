import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import ApiDocsView from '../ApiDocsView.vue'

const { locale, copyToClipboard } = vi.hoisted(() => ({
  locale: { value: 'zh' },
  copyToClipboard: vi.fn(),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ locale }),
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    siteName: 'Sub2API',
    apiBaseUrl: 'https://api-key.cc',
    cachedPublicSettings: {
      site_name: 'Sub2API',
      api_base_url: 'https://api-key.cc',
    },
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard }),
}))

function mountDocs() {
  return mount(ApiDocsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: true,
      },
    },
  })
}

describe('user ApiDocsView Seedance reference', () => {
  beforeEach(() => {
    locale.value = 'zh'
    copyToClipboard.mockReset()
  })

  it('renders the detailed Chinese Seedance request and lifecycle contract', () => {
    const text = mountDocs().text()

    expect(text).toContain('Seedance 2.0 / 2.5 异步视频接口')
    expect(text).toContain('创建请求字段')
    expect(text).toContain('content 元素、role 与素材编号')
    expect(text).toContain('视频编辑或延长：源视频作为视频1')
    expect(text).toContain('refund_status')
    expect(text).toContain('reference_video_duration_required')
    expect(text).toContain('invalid_video_resolution')
    expect(text).toContain('seedance-2.5')
    expect(text).toContain('支持纯音频参考')
    expect(text).toContain('支持真人人脸输入')
    expect(text).toContain('POST 创建不是幂等重试入口')
    expect(text).toContain('同步校验失败响应示例')
    expect(text).toContain('seedance / agent')
    expect(text).toContain('不要传 n')
    expect(text).not.toContain('"n": 1')
    expect(text).not.toContain('Seedance 使用 video 分组')
  })

  it('keeps the English Seedance documentation equally complete', () => {
    locale.value = 'en'
    const text = mountDocs().text()

    expect(text).toContain('Seedance 2.0 / 2.5 Async Video API')
    expect(text).toContain('Create Request Fields')
    expect(text).toContain('content Items, Roles, And Reference Numbering')
    expect(text).toContain('Video edit or extension: source is Video 1')
    expect(text).toContain('Common Error Codes')
    expect(text).toContain('POST creation is not an idempotent retry endpoint')
    expect(text).toContain('Synchronous validation error')
    expect(text).toContain('Seedance 2.5')
    expect(text).toContain('support real-person face input')
  })
})
