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
    expect(text).toContain('本地媒体随视频请求上传')
    expect(text).toContain('已经是 http:// 或 https:// 的输入 URL 会原样提交上游，本站不会下载、缓存或重新上传这些公网输入。')
    expect(text).toContain('attachment://N 顺序映射')
    expect(text).toContain('attachment://0')
    expect(text).toContain('公网输入保持原样')
    expect(text).toContain('一次请求：本地图片 + 本地视频 + 本地音频 + 公网图片')
    expect(text).toContain('temporary_asset_quota_exceeded')
    expect(text).toContain('https://api-key.cc/media/8db0d973-c281-4b6e-a6d7-550f2bcc2b31/asset.mp4')
    expect(text).not.toContain('POST /api/v1/agent/assets')
    expect(text).not.toContain('GET /api/v1/agent/assets')
    expect(text).not.toContain('所有已认证分组')
    expect(text).not.toContain('公网根地址')
    expect(text).not.toContain('生成结果也会转存')
    expect(text).not.toContain('Jingyu')
    expect(text).not.toContain('Aigod')
    expect(text).not.toContain('File Service')
    expect(text).not.toContain('供应商')
    expect(text).not.toContain('上游账号')
    expect(text).not.toContain('https://cdn.example.com/output.mp4')
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
    expect(text).toContain('Local Media In A Video Request')
    expect(text).toContain('attachment://N Order Mapping')
    expect(text).toContain('Public inputs stay unchanged')
    expect(text).toContain('One Request: Local Image + Video + Audio + Public Image')
    expect(text).not.toContain('POST /api/v1/agent/assets')
    expect(text).not.toContain('GET /api/v1/agent/assets')
    expect(text).not.toContain('Every authenticated group')
    expect(text).not.toContain('Public base URL')
    expect(text).not.toContain('Generated results are rehosted too')
    expect(text).not.toContain('Jingyu')
    expect(text).not.toContain('Aigod')
    expect(text).not.toContain('File Service')
    expect(text).not.toContain('supplier')
    expect(text).not.toContain('upstream account')
    expect(text).not.toContain('https://cdn.example.com/output.mp4')
  })
})
