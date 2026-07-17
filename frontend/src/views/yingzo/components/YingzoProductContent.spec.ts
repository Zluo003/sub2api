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
      version: '0.2.0',
      size_bytes: 1,
      artifacts: {
        openai: { host_family: 'openai', package_filename: 'yingzo-openai-0.2.0.tar.gz', size_bytes: 1 },
        claude: { host_family: 'claude', package_filename: 'yingzo-claude-0.2.0.tar.gz', size_bytes: 1 },
      },
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
      version: '0.2.0',
      host_family: 'claude',
      download_url: 'https://api-key.cc/package.tar.gz',
      expires_at: new Date().toISOString(),
      prompt: 'claude-only prompt',
    })
    const wrapper = mount(YingzoProductContent, {
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
    })
    await flushPromises()

    const installButtons = wrapper.findAll('button.yingzo-copy-button')
    expect(installButtons).toHaveLength(4)
    expect(installButtons[0].text()).toContain('复制给 ChatGPT Work')
    expect(installButtons[1].text()).toContain('复制给 Codex')
    expect(installButtons[2].text()).toContain('复制给 Claude Cowork')
    expect(installButtons[3].text()).toContain('复制给 Claude Code')

    await installButtons[3].trigger('click')
    await flushPromises()

    expect(mocks.createInstructions).toHaveBeenCalledWith('claude-code')
    expect(mocks.writeText).toHaveBeenCalledWith('claude-only prompt')
    expect(wrapper.text()).not.toContain('SHA-256')
  })

  it('does not copy a prompt when the server returns another host', async () => {
    mocks.createInstructions.mockResolvedValue({
      host: 'codex',
      version: '0.2.0',
      host_family: 'openai',
      download_url: 'https://api-key.cc/package.tar.gz',
      expires_at: new Date().toISOString(),
      prompt: 'wrong-host prompt',
    })
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const wrapper = mount(YingzoProductContent, {
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
    })
    await flushPromises()

    const installButtons = wrapper.findAll('button.yingzo-copy-button')
    await installButtons[3].trigger('click')
    await flushPromises()

    expect(mocks.writeText).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('生成安装提示词失败')
    expect(consoleError).toHaveBeenCalledWith(expect.objectContaining({ message: expect.stringContaining('Install host mismatch') }))
    consoleError.mockRestore()
  })
})
