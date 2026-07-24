import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const apiMocks = vi.hoisted(() => ({
  getAgentModels: vi.fn(),
  syncAgentModels: vi.fn(),
  setAgentPlatformRate: vi.fn(),
  updateAgentModel: vi.fn(),
  deleteAgentModel: vi.fn()
}))

const notificationMocks = vi.hoisted(() => ({
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: apiMocks
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => notificationMocks
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

import AgentModelCatalogEditor from '../AgentModelCatalogEditor.vue'
import type { AgentGroupModel, AgentModelConfig } from '@/api/admin/groups'

const groupId = 77

function model(overrides: Partial<AgentGroupModel>): AgentGroupModel {
  return {
    id: 1,
    group_id: groupId,
    platform: 'openai',
    model_code: 'model-1',
    media_type: 'text',
    enabled: true,
    available: true,
    excluded: false,
    discovered_at: '2026-07-24T00:00:00Z',
    last_seen_at: '2026-07-24T00:00:00Z',
    prices: [],
    ...overrides
  }
}

function config(models: AgentGroupModel[] = []): AgentModelConfig {
  return { platform_rates: [], models }
}

async function mountEditor(initialConfig: AgentModelConfig) {
  apiMocks.getAgentModels.mockResolvedValue(initialConfig)
  const wrapper = mount(AgentModelCatalogEditor, {
    props: { groupId },
    global: {
      stubs: {
        Icon: { template: '<span />' }
      }
    }
  })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  vi.clearAllMocks()
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('AgentModelCatalogEditor', () => {
  it('loads and synchronizes the persisted account model catalogue', async () => {
    const wrapper = await mountEditor(config([
      model({ id: 1, model_code: 'gpt-5.4', media_type: 'text' })
    ]))
    apiMocks.syncAgentModels.mockResolvedValue(config([
      model({ id: 2, platform: 'seedance', model_code: 'seedance-custom', media_type: 'video' })
    ]))

    expect(apiMocks.getAgentModels).toHaveBeenCalledWith(groupId)
    expect(wrapper.text()).toContain('gpt-5.4')

    await wrapper.get('[data-testid="sync-agent-models"]').trigger('click')
    await flushPromises()

    expect(apiMocks.syncAgentModels).toHaveBeenCalledWith(groupId)
    expect(wrapper.text()).toContain('seedance-custom')
    expect(wrapper.text()).not.toContain('gpt-5.4')
  })

  it('saves each configured platform multiplier without losing later draft values', async () => {
    const wrapper = await mountEditor(config())
    apiMocks.setAgentPlatformRate
      .mockResolvedValueOnce({
        platform_rates: [{ group_id: groupId, platform: 'openai', rate_multiplier: 1.25 }],
        models: []
      })
      .mockResolvedValueOnce({
        platform_rates: [
          { group_id: groupId, platform: 'openai', rate_multiplier: 1.25 },
          { group_id: groupId, platform: 'gemini', rate_multiplier: 0 }
        ],
        models: []
      })

    await wrapper.get('[data-testid="agent-rate-openai"]').setValue('1.25')
    await wrapper.get('[data-testid="agent-rate-gemini"]').setValue('0')
    await wrapper.get('[data-testid="save-agent-platform-rates"]').trigger('click')
    await flushPromises()

    expect(apiMocks.setAgentPlatformRate).toHaveBeenNthCalledWith(1, groupId, 'openai', 1.25)
    expect(apiMocks.setAgentPlatformRate).toHaveBeenNthCalledWith(2, groupId, 'gemini', 0)
    expect(apiMocks.setAgentPlatformRate).toHaveBeenCalledTimes(2)
  })

  it('saves image prices by model and preserves an explicit zero price', async () => {
    const imageModel = model({
      id: 11,
      model_code: 'gpt-image-custom',
      media_type: 'image',
      prices: [
        { resolution: '1K', billing_unit: 'image', unit_price: 0.1 },
        { resolution: '4K', billing_unit: 'image', unit_price: 0 }
      ]
    })
    const imageConfig = config([imageModel])
    const wrapper = await mountEditor(imageConfig)
    apiMocks.updateAgentModel.mockResolvedValue(imageConfig)

    await wrapper.get('[data-testid="agent-model-price-11-2K"]').setValue('0.2')
    await wrapper.get('[data-testid="save-agent-model-11"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateAgentModel).toHaveBeenCalledWith(groupId, 11, {
      media_type: 'image',
      enabled: true,
      prices: [
        { resolution: '1K', unit_price: 0.1 },
        { resolution: '2K', unit_price: 0.2 },
        { resolution: '4K', unit_price: 0 }
      ]
    })
  })

  it('adds and saves arbitrary video resolution pricing for a synchronized model', async () => {
    const videoModel = model({
      id: 12,
      platform: 'seedance',
      model_code: 'seedance-custom',
      media_type: 'video'
    })
    const videoConfig = config([videoModel])
    const wrapper = await mountEditor(videoConfig)
    apiMocks.updateAgentModel.mockResolvedValue(videoConfig)

    await wrapper.get('[data-testid="add-agent-video-price-12"]').trigger('click')
    await wrapper.get('[data-testid="agent-video-resolution-12-0"]').setValue('1440p')
    await wrapper.get('[data-testid="agent-video-price-12-0"]').setValue('0.08')
    await wrapper.get('[data-testid="save-agent-model-12"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateAgentModel).toHaveBeenCalledWith(groupId, 12, {
      media_type: 'video',
      enabled: true,
      prices: [{ resolution: '1440p', unit_price: 0.08 }]
    })
  })

  it('excludes a model and applies the returned catalogue immediately', async () => {
    const wrapper = await mountEditor(config([
      model({ id: 13, model_code: 'remove-me', media_type: 'image' })
    ]))
    apiMocks.deleteAgentModel.mockResolvedValue(config())
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    await wrapper.get('[data-testid="exclude-agent-model-13"]').trigger('click')
    await flushPromises()

    expect(apiMocks.deleteAgentModel).toHaveBeenCalledWith(groupId, 13)
    expect(wrapper.find('[data-testid="agent-model-13"]').exists()).toBe(false)
  })
})
