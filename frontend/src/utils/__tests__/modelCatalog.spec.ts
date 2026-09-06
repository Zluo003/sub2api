import { describe, expect, it } from 'vitest'
import { getModelIdentity } from '@/utils/modelCatalog'

describe('modelCatalog', () => {
  it.each([
    ['claude-opus-4-6', 'antigravity', 'Anthropic'],
    ['gpt-5.4', 'openai', 'OpenAI'],
    ['gemini-3-flash', 'antigravity', 'Google'],
    ['grok-4', 'grok', 'xAI'],
    ['deepseek-reasoner', 'openai', 'DeepSeek'],
    ['qwen3-coder', 'openai', 'Alibaba Cloud'],
    ['seedance-1-5-pro', 'seedance', 'ByteDance']
  ])('infers %s as %s', (model, platform, vendor) => {
    expect(getModelIdentity(model, platform).vendor).toBe(vendor)
  })

  it('falls back to the platform while keeping a useful description', () => {
    const identity = getModelIdentity('custom-model', 'acme')
    expect(identity.vendor).toBe('acme')
    expect(identity.descriptionZh).toContain('acme')
  })

  it.each([
    ['seedance-2.0', '满血视频模型', '9 张参考图', '4K'],
    ['seedance-2.0-fast', '极速视频模型', '3 个参考视频', '720p'],
    ['seedance-2.5', '新一代视频模型', '30 张参考图', '纯音频参考']
  ])('uses model-specific official Seedance copy for %s', (model, edition, referenceCapability, outputCapability) => {
    const identity = getModelIdentity(model, 'seedance')

    expect(identity.vendor).toBe('ByteDance')
    expect(identity.descriptionZh).toContain(edition)
    expect(identity.descriptionZh).toContain('官方直连')
    expect(identity.descriptionZh).toContain(referenceCapability)
    expect(identity.descriptionZh).toContain(outputCapability)
    expect(identity.descriptionEn).toContain('direct official access')
    expect(identity.descriptionZh).not.toContain('豆包/Seed 系列模型')
  })

  it.each([
    ['minimax-h3', 'MiniMax', '2K'],
    ['minimax-h3-max', 'MiniMax', '12 张参考图'],
    ['hailuo-3', 'MiniMax', '2K'],
    ['minimax-hailuo-3-max', 'MiniMax', '12 张参考图'],
    ['wan-3', 'Alibaba Cloud', '2–30 秒'],
    ['wan3', 'Alibaba Cloud', '2–30 秒'],
    ['wan-3.0', 'Alibaba Cloud', '2–30 秒']
  ])('uses model-specific copy for the mikuapi video model %s', (model, vendor, capability) => {
    const identity = getModelIdentity(model, 'seedance')

    expect(identity.vendor).toBe(vendor)
    expect(identity.descriptionZh).toContain(capability)
    // The seedance platform falls back to ByteDance, which would be wrong here.
    expect(identity.descriptionZh).not.toContain('豆包/Seed 系列模型')
  })
})
