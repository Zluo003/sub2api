import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import YingzoProductContent from './YingzoProductContent.vue'

const mocks = vi.hoisted(() => ({
  createInstructions: vi.fn(),
  getLatestRelease: vi.fn(),
  writeText: vi.fn(),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isAuthenticated: true }),
}))

vi.mock('@/api/yingzo', () => ({
  createYingzoInstallInstructions: mocks.createInstructions,
  getLatestYingzoRelease: mocks.getLatestRelease,
}))

describe('YingzoProductContent install actions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getLatestRelease.mockResolvedValue({
      version: '0.1.5',
      size_bytes: 1,
      sha256: 'a'.repeat(64),
      min_codex_version: '0.143.0',
      min_claude_version: '2.1.201',
    })
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: mocks.writeText },
    })
  })

  it('copies a Claude Code prompt using the explicit Claude host', async () => {
    mocks.createInstructions.mockResolvedValue({
      host: 'claude-code',
      version: '0.1.5',
      download_url: 'https://api-key.cc/package.tar.gz',
      expires_at: new Date().toISOString(),
      prompt: 'claude-only prompt',
    })
    const wrapper = mount(YingzoProductContent)
    await flushPromises()

    const installButtons = wrapper.findAll('button.yingzo-copy-button')
    expect(installButtons).toHaveLength(2)
    expect(installButtons[0].text()).toContain('复制给 Codex')
    expect(installButtons[1].text()).toContain('复制给 Claude Code')

    await installButtons[1].trigger('click')
    await flushPromises()

    expect(mocks.createInstructions).toHaveBeenCalledWith('claude-code')
    expect(mocks.writeText).toHaveBeenCalledWith('claude-only prompt')
    expect(wrapper.text()).not.toContain('SHA-256')
  })

  it('does not copy a prompt when the server returns another host', async () => {
    mocks.createInstructions.mockResolvedValue({
      host: 'codex',
      version: '0.1.5',
      download_url: 'https://api-key.cc/package.tar.gz',
      expires_at: new Date().toISOString(),
      prompt: 'wrong-host prompt',
    })
    const wrapper = mount(YingzoProductContent)
    await flushPromises()

    const installButtons = wrapper.findAll('button.yingzo-copy-button')
    await installButtons[1].trigger('click')
    await flushPromises()

    expect(mocks.writeText).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('生成安装提示词失败')
  })
})
