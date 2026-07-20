import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import YingzoView from '../YingzoView.vue'

const mocks = vi.hoisted(() => ({
  list: vi.fn(), settings: vi.fn(), createDraft: vi.fn(), uploadArtifact: vi.fn(),
  replaceArtifact: vi.fn(), deleteArtifact: vi.fn(), updateSettings: vi.fn(),
  publish: vi.fn(), promote: vi.fn(), rollback: vi.fn(), disable: vi.fn(), verifyProof: vi.fn(),
}))

vi.mock('@/api/yingzo', () => ({
  listYingzoReleases: mocks.list,
  getYingzoAdminSettings: mocks.settings,
  createYingzoReleaseDraft: mocks.createDraft,
  uploadYingzoReleaseArtifact: mocks.uploadArtifact,
  replaceYingzoReleaseArtifact: mocks.replaceArtifact,
  deleteYingzoReleaseArtifact: mocks.deleteArtifact,
  updateYingzoAdminSettings: mocks.updateSettings,
  publishYingzoRelease: mocks.publish,
  promoteYingzoRelease: mocks.promote,
  rollbackYingzoRelease: mocks.rollback,
  disableYingzoRelease: mocks.disable,
  verifyYingzoReleaseProof: mocks.verifyProof,
}))

const global = {
  stubs: {
    AppLayout: { template: '<main><slot /></main>' },
    RouterLink: { template: '<a><slot /></a>' },
    Icon: true,
  },
}

function schema2Draft() {
  return {
    id: 'release-1', version: '0.3.0', status: 'draft', channel: 'prerelease',
    distribution_schema_version: 2, runtime_protocol: 1, artifact_matrix: [],
    min_codex_version: '0.143.0', min_claude_version: '2.1.201',
  }
}

describe('Yingzo release administration', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.list.mockResolvedValue([])
    mocks.settings.mockResolvedValue({ public_origin: 'https://api-key.cc', effective_origin: 'https://api-key.cc', release_storage: '/data/releases' })
    mocks.createDraft.mockResolvedValue(schema2Draft())
    mocks.uploadArtifact.mockResolvedValue({ id: 'artifact-1' })
  })

  it('creates a schema 2 prerelease draft before accepting artifacts', async () => {
    const wrapper = mount(YingzoView, { global })
    await flushPromises()

    await wrapper.find('form input[placeholder="0.3.0"]').setValue('0.3.1')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(mocks.createDraft).toHaveBeenCalledWith(expect.objectContaining({
      version: '0.3.1', channel: 'prerelease', distribution_schema_version: 2,
      runtime_protocol: 1,
      compatibility: { platforms: ['macos-arm64', 'macos-x64', 'windows-x64'], artifact_count: 8 },
    }))
  })

  it('renders the exact eight-artifact matrix and uploads one target at a time', async () => {
    mocks.list.mockResolvedValue([schema2Draft()])
    const wrapper = mount(YingzoView, { global })
    await flushPromises()

    const files = wrapper.findAll('input[type="file"]')
    expect(files).toHaveLength(9)
    expect(wrapper.text()).toContain('OpenAI / Codex · macOS')
    expect(wrapper.text()).toContain('Runtime · Windows x64')

    const archive = new File(['openai'], 'yingzo-openai-macos-0.3.0.tar.gz', { type: 'application/gzip' })
    Object.defineProperty(files[0].element, 'files', { configurable: true, value: [archive] })
    await files[0].trigger('change')
    const upload = wrapper.findAll('button').find((button) => button.text() === '上传')
    expect(upload).toBeDefined()
    await upload!.trigger('click')
    await flushPromises()

    expect(mocks.uploadArtifact).toHaveBeenCalledWith('release-1', expect.objectContaining({
      file: archive, artifact_kind: 'host_package', target: 'openai', os: 'macos', arch: 'any', runtime_protocol: 1,
    }))
  })

  it('submits the CI proof envelope as one file without trusting client status', async () => {
    mocks.list.mockResolvedValue([schema2Draft()])
    mocks.verifyProof.mockResolvedValue({ verified: true })
    const wrapper = mount(YingzoView, { global })
    await flushPromises()

    const files = wrapper.findAll('input[type="file"]')
    const proof = new File([JSON.stringify({ algorithm: 'Ed25519', key_id: 'release-2026', manifest_base64: 'bWFuaWZlc3Q=', signature_base64: 'c2ln' })], 'yingzo-release-0.3.0.proof.json', { type: 'application/json' })
    Object.defineProperty(files[8].element, 'files', { configurable: true, value: [proof] })
    await files[8].trigger('change')
    await wrapper.findAll('button').find((button) => button.text() === '验证发行证明')!.trigger('click')
    await vi.waitFor(() => {
      expect(mocks.verifyProof).toHaveBeenCalledWith('release-1', {
        algorithm: 'Ed25519', key_id: 'release-2026', manifest_base64: 'bWFuaWZlc3Q=', signature_base64: 'c2ln',
      })
    })
  })

  it('shows normalized artifact validation failures', async () => {
    mocks.list.mockResolvedValue([schema2Draft()])
    mocks.uploadArtifact.mockRejectedValue({ error: { code: 'invalid_release_archive', message: 'package is missing .codex-plugin/plugin.json' } })
    const wrapper = mount(YingzoView, { global })
    await flushPromises()

    const file = wrapper.find('input[type="file"]')
    Object.defineProperty(file.element, 'files', { configurable: true, value: [new File(['x'], 'bad.tar.gz')] })
    await file.trigger('change')
    await wrapper.findAll('button').find((button) => button.text() === '上传')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('package is missing .codex-plugin/plugin.json')
  })

  it('promotes the same published prerelease without creating another release', async () => {
    mocks.list.mockResolvedValue([{ ...schema2Draft(), status: 'published' }])
    mocks.promote.mockResolvedValue({ ...schema2Draft(), status: 'published', channel: 'stable' })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mount(YingzoView, { global })
    await flushPromises()

    const promote = wrapper.findAll('button').find((button) => button.text() === '提升为稳定版')
    expect(promote).toBeDefined()
    await promote!.trigger('click')
    await flushPromises()

    expect(mocks.promote).toHaveBeenCalledWith('release-1')
    expect(mocks.createDraft).not.toHaveBeenCalled()
  })

  it('does not render the legacy artifact list for a published schema 2 release', async () => {
    mocks.list.mockResolvedValue([{
      ...schema2Draft(),
      status: 'published',
      artifact_matrix: [{
        id: 'artifact-1', artifact_kind: 'host_package', target: 'openai', os: 'macos', arch: 'any',
        package_filename: 'yingzo-openai-macos-0.3.0.tar.gz', size_bytes: 1024,
        validation_status: 'validated', signature_status: 'verified',
      }],
    }])
    const wrapper = mount(YingzoView, { global })
    await flushPromises()

    expect(wrapper.findAll('.artifact-slot')).toHaveLength(8)
    expect(wrapper.findAll('.legacy-artifact')).toHaveLength(0)
  })
})
