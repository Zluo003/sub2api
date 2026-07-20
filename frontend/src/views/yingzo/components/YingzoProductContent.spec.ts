import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import YingzoProductContent from './YingzoProductContent.vue'

const mocks = vi.hoisted(() => ({ createInstructions: vi.fn(), getLatestRelease: vi.fn(), writeText: vi.fn() }))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAuthenticated: true }) }))
vi.mock('@/api/yingzo', () => ({
  createYingzoInstallInstructions: mocks.createInstructions,
  getLatestYingzoRelease: mocks.getLatestRelease,
}))

function release(channel = 'stable', schemaVersion = 2) {
  return {
    version: schemaVersion >= 2 ? '0.3.0' : '0.2.4', channel, distribution_schema_version: schemaVersion,
    runtime_protocol: schemaVersion >= 2 ? 1 : 0,
    min_codex_version: '0.143.0', min_claude_version: '2.1.201', artifact_matrix: [],
  }
}

describe('YingzoProductContent install actions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getLatestRelease.mockResolvedValue(release())
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: mocks.writeText } })
  })

  it('requests the selected host with an explicit platform and stable channel', async () => {
    mocks.createInstructions.mockResolvedValue({
      host: 'claude-code', version: '0.3.0', host_family: 'claude', channel: 'stable',
      download_url: 'https://api-key.cc/package.zip', expires_at: new Date().toISOString(), prompt: 'claude-only prompt',
    })
    const wrapper = mount(YingzoProductContent, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await flushPromises()

    const installButtons = wrapper.findAll('button.yingzo-copy-button')
    expect(installButtons).toHaveLength(5)
    expect(installButtons.map((button) => button.text()).join(' ')).toContain('Claude Desktop')
    await installButtons[4].trigger('click')
    await flushPromises()

    expect(mocks.createInstructions).toHaveBeenCalledWith({ host: 'claude-code', os: 'macos', arch: 'arm64', channel: 'stable', runtime_capability: 'unknown' })
    expect(mocks.writeText).toHaveBeenCalledWith('claude-only prompt')
    expect(wrapper.text()).not.toContain('SHA-256')
  })

  it('loads prerelease only after the user opts in', async () => {
    mocks.getLatestRelease.mockImplementation(async (channel: string) => channel === 'prerelease'
      ? release(channel)
      : release(channel))
    const wrapper = mount(YingzoProductContent, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await flushPromises()
    expect(mocks.getLatestRelease).toHaveBeenCalledWith('stable')

    await wrapper.find('.yingzo-channel-toggle input').setValue(true)
    await flushPromises()
    expect(mocks.getLatestRelease).toHaveBeenCalledWith('prerelease')
    expect(wrapper.text()).toContain('预发布版 0.3.0')
    expect(wrapper.text()).not.toContain('不能提升为稳定版')
  })

  it('does not offer Claude Desktop for a legacy schema 1 stable release', async () => {
    mocks.getLatestRelease.mockResolvedValue(release('stable', 1))
    const wrapper = mount(YingzoProductContent, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await flushPromises()

    const installButtons = wrapper.findAll('button.yingzo-copy-button')
    expect(installButtons).toHaveLength(4)
    expect(installButtons.map((button) => button.text()).join(' ')).not.toContain('Claude Desktop')
  })

  it('does not copy a prompt when the server returns another host', async () => {
    mocks.createInstructions.mockResolvedValue({ host: 'codex', version: '0.3.0', host_family: 'openai', download_url: 'https://api-key.cc/package.zip', expires_at: new Date().toISOString(), prompt: 'wrong-host prompt' })
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const wrapper = mount(YingzoProductContent, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await flushPromises()

    await wrapper.findAll('button.yingzo-copy-button')[4].trigger('click')
    await flushPromises()
    expect(mocks.writeText).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('生成安装提示词失败：Install host mismatch')
    consoleError.mockRestore()
  })

  it('shows the server platform error instead of hiding it behind a generic download error', async () => {
    mocks.createInstructions.mockRejectedValue({ error: 'Windows x64 package is unavailable for this host' })
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const wrapper = mount(YingzoProductContent, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await flushPromises()

    await wrapper.findAll('button.yingzo-copy-button')[4].trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Windows x64 package is unavailable for this host')
    consoleError.mockRestore()
  })
})
