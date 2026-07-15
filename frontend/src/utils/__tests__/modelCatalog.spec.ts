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
})
