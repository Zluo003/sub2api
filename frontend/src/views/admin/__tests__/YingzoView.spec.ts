import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import YingzoView from '../YingzoView.vue'

const mocks = vi.hoisted(() => ({
  list: vi.fn(), settings: vi.fn(), createDraft: vi.fn(), uploadArtifact: vi.fn(),
  replaceArtifact: vi.fn(), deleteArtifact: vi.fn(), updateSettings: vi.fn(),
  publish: vi.fn(), promote: vi.fn(), rollback: vi.fn(), disable: vi.fn(),
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

function artifactMatrix(schemaVersion = 2) {
  const slots = [
    ['openai', 'macos', 'any'],
    ['openai', 'windows', 'x64'],
    ['claude-code', 'macos', 'any'],
    ['claude-code', 'windows', 'x64'],
    ['claude-desktop', 'any', 'any'],
    ['runtime', 'macos', 'arm64'],
    ['runtime', 'windows', 'x64'],
  ]
  if (schemaVersion === 2) slots.splice(6, 0, ['runtime', 'macos', 'x64'])
  return slots.map(([target, os, arch], index) => ({
    id: `artifact-${index}`, artifact_kind: target === 'runtime' ? 'runtime_installer' : 'host_package',
    target, os, arch, package_filename: `artifact-${index}`, size_bytes: 1024,
  }))
}

function schema3Draft() {
  return {
    id: 'release-3', version: '0.3.0', status: 'draft', channel: 'prerelease',
    distribution_schema_version: 3, runtime_protocol: 1, artifact_matrix: [],
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

  it('creates a schema 3 prerelease draft with the current seven-artifact contract', async () => {
    const wrapper = mount(YingzoView, { global })
    await flushPromises()

    await wrapper.find('form input[placeholder="0.3.0"]').setValue('0.3.1')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(mocks.createDraft).toHaveBeenCalledWith(expect.objectContaining({
      version: '0.3.1', channel: 'prerelease', distribution_schema_version: 3,
      runtime_protocol: 1,
      compatibility: { platforms: ['macos-arm64', 'windows-x64'], artifact_count: 7 },
    }))
  })

  it('renders seven current schema 3 slots and does not offer macOS x64 Runtime', async () => {
    mocks.list.mockResolvedValue([schema3Draft()])
    const wrapper = mount(YingzoView, { global })
    await flushPromises()

    expect(wrapper.findAll('input[type="file"]')).toHaveLength(7)
    expect(wrapper.text()).toContain('Runtime · macOS arm64')
    expect(wrapper.text()).not.toContain('Runtime · macOS Intel')
  })

  it('prefers server-declared schema 3 requirements over the default matrix', async () => {
    mocks.list.mockResolvedValue([{
      ...schema3Draft(),
      required_artifacts: [{
        key: 'runtime-macos-arm64-custom', label: 'Runtime · macOS arm64（自定义）', artifact_kind: 'runtime_installer',
        target: 'runtime', os: 'macos', arch: 'arm64', package_filename: 'custom-runtime-{version}.dmg', format: 'dmg',
      }],
    }])
    const wrapper = mount(YingzoView, { global })
    await flushPromises()

    expect(wrapper.findAll('input[type="file"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('Runtime · macOS arm64（自定义）')
    expect(wrapper.text()).toContain('custom-runtime-0.3.0.dmg')
  })

  it('renders the exact eight-artifact matrix and uploads one target at a time', async () => {
    mocks.list.mockResolvedValue([schema2Draft()])
    const wrapper = mount(YingzoView, { global })
    await flushPromises()

    const files = wrapper.findAll('input[type="file"]')
    expect(files).toHaveLength(8)
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

  it('enables publishing only after all eight slots contain an artifact', async () => {
    mocks.list.mockResolvedValue([{ ...schema2Draft(), artifact_matrix: artifactMatrix().slice(0, -1) }])
    const wrapper = mount(YingzoView, { global })
    await flushPromises()

    const publish = wrapper.findAll('button').find((button) => button.text() === '发布')
    expect(publish?.attributes('disabled')).toBeDefined()

    mocks.list.mockResolvedValue([{ ...schema2Draft(), artifact_matrix: artifactMatrix() }])
    await wrapper.findAll('button').find((button) => button.attributes('title') === '刷新')!.trigger('click')
    await flushPromises()

    expect(wrapper.findAll('button').find((button) => button.text() === '发布')?.attributes('disabled')).toBeUndefined()
    await wrapper.findAll('button').find((button) => button.text() === '发布')!.trigger('click')
    await flushPromises()
    expect(mocks.publish).toHaveBeenCalledWith('release-1')
  })

  it('shows the server filename or slot error when an upload is rejected', async () => {
    mocks.list.mockResolvedValue([schema2Draft()])
    mocks.uploadArtifact.mockRejectedValue({ error: { code: 'invalid_package_filename', message: 'package filename must be yingzo-openai-macos-0.3.0.tar.gz' } })
    const wrapper = mount(YingzoView, { global })
    await flushPromises()

    const file = wrapper.find('input[type="file"]')
    Object.defineProperty(file.element, 'files', { configurable: true, value: [new File(['x'], 'bad.tar.gz')] })
    await file.trigger('change')
    await wrapper.findAll('button').find((button) => button.text() === '上传')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('package filename must be yingzo-openai-macos-0.3.0.tar.gz')
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

  it('allows a published prerelease to be promoted when legacy stable eligibility is false', async () => {
    mocks.list.mockResolvedValue([{
      ...schema2Draft(), status: 'published', stable_eligible: false, artifact_matrix: artifactMatrix(),
    }])
    const wrapper = mount(YingzoView, { global })
    await flushPromises()

    const promote = wrapper.findAll('button').find((button) => button.text() === '提升为稳定版')
    expect(promote?.attributes('disabled')).toBeUndefined()
  })

  it('does not render the legacy artifact list for a published schema 2 release', async () => {
    mocks.list.mockResolvedValue([{
      ...schema2Draft(),
      status: 'published',
      artifact_matrix: [{
        id: 'artifact-1', artifact_kind: 'host_package', target: 'openai', os: 'macos', arch: 'any',
        package_filename: 'yingzo-openai-macos-0.3.0.tar.gz', size_bytes: 1024,
      }],
    }])
    const wrapper = mount(YingzoView, { global })
    await flushPromises()

    expect(wrapper.findAll('.artifact-slot')).toHaveLength(8)
    expect(wrapper.findAll('.legacy-artifact')).toHaveLength(0)
  })
})
