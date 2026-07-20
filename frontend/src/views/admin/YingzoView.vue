<template>
  <AppLayout>
    <div class="yingzo-admin space-y-8">
      <header class="flex flex-col justify-between gap-4 border-b border-gray-200 pb-6 dark:border-dark-700 md:flex-row md:items-end">
        <div>
          <h1 class="text-2xl font-semibold text-gray-950 dark:text-white">Yingzo（影作）发行管理</h1>
          <p class="mt-2 max-w-3xl text-sm leading-6 text-gray-500 dark:text-gray-400">管理稳定版与预发布版、五个宿主包和三个 Runtime 安装器。二进制保存在服务器持久卷，数据库只记录发行元数据。</p>
        </div>
        <router-link to="/yingzo" class="btn btn-secondary btn-sm">查看产品页</router-link>
      </header>

      <section class="grid gap-8 xl:grid-cols-[minmax(0,.8fr)_minmax(520px,1.2fr)]">
        <div class="border-t-2 border-gray-900 pt-5 dark:border-white">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">通信与存储</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">生产环境建议使用固定 HTTPS 域名；发行文件写入服务器持久卷。</p>
          <div class="mt-5 flex flex-col gap-3 sm:flex-row">
            <input v-model="publicOrigin" class="input min-w-0 flex-1" placeholder="https://api-key.cc" autocomplete="off" />
            <button type="button" class="btn btn-primary" :disabled="savingSettings" @click="saveSettings">{{ savingSettings ? '保存中' : '保存域名' }}</button>
          </div>
          <dl class="mt-4 grid gap-3 text-xs text-gray-500 dark:text-gray-400">
            <div><dt class="font-medium text-gray-700 dark:text-gray-300">当前生效</dt><dd class="mt-1 break-all">{{ settings?.effective_origin || '读取中' }}</dd></div>
            <div><dt class="font-medium text-gray-700 dark:text-gray-300">本地发行卷</dt><dd class="mt-1 break-all">{{ settings?.release_storage || '读取中' }}</dd></div>
          </dl>
        </div>

        <form class="border-t-2 border-[#df5b48] pt-5" @submit.prevent="createDraft">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">创建发行草稿</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">先保存版本元数据，再逐项上传八个签名产物。缺少任何一项都不能发布。</p>
          <div class="mt-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <label class="field-label">版本<input v-model.trim="draftForm.version" required class="input mt-1 w-full" placeholder="0.3.0" /></label>
            <label class="field-label">通道<select v-model="draftForm.channel" class="input mt-1 w-full"><option value="prerelease">预发布</option><option value="stable">稳定版</option></select></label>
            <label class="field-label">Runtime 协议<input v-model.number="draftForm.runtimeProtocol" required min="1" type="number" class="input mt-1 w-full" /></label>
            <label class="field-label">最低 Codex 版本<input v-model.trim="draftForm.minCodex" class="input mt-1 w-full" placeholder="0.143.0" /></label>
            <label class="field-label">最低 Claude Code 版本<input v-model.trim="draftForm.minClaude" class="input mt-1 w-full" placeholder="2.1.201" /></label>
            <label class="field-label sm:col-span-2 lg:col-span-3">更新说明<textarea v-model="draftForm.notes" class="input mt-1 min-h-20 w-full py-2" /></label>
          </div>
          <button type="submit" class="btn btn-primary mt-4 inline-flex items-center gap-2" :disabled="creatingDraft">
            <Icon name="plus" size="sm" />{{ creatingDraft ? '创建中' : '创建八项发行草稿' }}
          </button>
        </form>
      </section>

      <p v-if="message" class="border-l-2 px-3 py-2 text-sm" :class="messageKind === 'error' ? 'border-red-500 bg-red-50 text-red-700 dark:bg-red-950/30 dark:text-red-300' : 'border-emerald-500 bg-emerald-50 text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-300'">{{ message }}</p>

      <section>
        <div class="mb-4 flex items-end justify-between gap-4">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">发行记录</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">稳定版和预发布版独立切换；旧 schema 1 版本继续保留下载与回滚能力。</p>
          </div>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" title="刷新" @click="loadAll"><Icon name="refresh" size="sm" /></button>
        </div>

        <div class="space-y-5">
          <article v-for="release in releases" :key="release.id || release.version" class="release-panel">
            <header class="flex flex-col justify-between gap-3 border-b border-gray-200 pb-4 dark:border-dark-700 md:flex-row md:items-center">
              <div class="flex flex-wrap items-center gap-3">
                <h3 class="text-lg font-semibold text-gray-950 dark:text-white">{{ release.version }}</h3>
                <span class="status-mark" :class="`status-${release.status}`">{{ statusLabel(release.status) }}</span>
                <span class="channel-mark">{{ channelLabel(release.channel) }}</span>
                <span class="text-xs text-gray-400">schema {{ release.distribution_schema_version || 1 }}</span>
              </div>
              <div class="flex flex-wrap gap-2">
                <button v-if="release.status === 'draft'" type="button" class="btn btn-primary btn-xs" :disabled="!canPublish(release)" @click="publish(release)">发布</button>
                <button v-if="release.status === 'published' && release.channel === 'prerelease'" type="button" class="btn btn-primary btn-xs" @click="promote(release)">提升为稳定版</button>
                <button v-if="release.status === 'superseded'" type="button" class="btn btn-secondary btn-xs" @click="rollback(release)">回滚至此版本</button>
                <button v-if="release.status !== 'disabled'" type="button" class="btn btn-danger btn-xs" @click="disable(release)">停用</button>
              </div>
            </header>

            <div v-if="(release.distribution_schema_version || 1) >= 2" class="mt-4 artifact-grid">
              <section v-for="slot in artifactSlots" :key="slot.key" class="artifact-slot">
                <div class="min-w-0">
                  <div class="flex items-center gap-2">
                    <strong class="text-sm text-gray-800 dark:text-gray-100">{{ slot.label }}</strong>
                    <span v-if="artifactReady(artifactForSlot(release, slot))" class="verified-mark">校验与签名通过</span>
                    <span v-else-if="artifactForSlot(release, slot)" class="unverified-mark">签名未确认</span>
                    <span v-else class="missing-mark">缺少</span>
                  </div>
                  <p class="mt-1 truncate text-xs text-gray-400" :title="expectedFilename(slot, release.version)">{{ artifactForSlot(release, slot)?.package_filename || expectedFilename(slot, release.version) }}</p>
                  <p v-if="artifactForSlot(release, slot)" class="mt-1 text-xs text-gray-500">{{ formatBytes(artifactForSlot(release, slot)?.size_bytes || 0) }}</p>
                </div>
                <div v-if="release.status === 'draft' && release.id" class="mt-3 space-y-2">
                  <input type="file" :accept="slot.accept" class="input w-full py-1.5 text-xs" @change="selectArtifactFile(release.id, slot.key, $event)" />
                  <div class="flex gap-2">
                    <button type="button" class="btn btn-secondary btn-xs" :disabled="!selectedArtifactFile(release.id, slot.key) || uploadingSlot === selectionKey(release.id, slot.key)" @click="uploadArtifact(release, slot)">{{ artifactForSlot(release, slot) ? '替换' : '上传' }}</button>
                    <button v-if="artifactForSlot(release, slot)?.id" type="button" class="btn btn-danger btn-xs" @click="removeArtifact(release, artifactForSlot(release, slot)!)">删除</button>
                  </div>
                </div>
              </section>
            </div>

            <section v-if="release.status === 'draft' && release.id && (release.distribution_schema_version || 1) >= 2" class="proof-panel">
              <div>
                <strong class="text-sm text-gray-800 dark:text-gray-100">Ed25519 发行证明</strong>
                <p class="mt-1 text-xs text-gray-500">八个文件上传完成后，直接上传 CI 生成的单个 <code>yingzo-release-版本.proof.json</code>。服务端会验证原始 manifest 字节、签名和八项文件哈希。</p>
              </div>
              <div class="proof-inputs">
                <input type="file" accept=".proof.json,application/json" class="input" @change="selectProofFile(release.id, $event)" />
                <button type="button" class="btn btn-secondary btn-sm" :disabled="!proofReady(release.id) || verifyingProof === release.id" @click="submitProof(release)">{{ verifyingProof === release.id ? '验签中' : '验证发行证明' }}</button>
              </div>
            </section>

            <div v-if="(release.distribution_schema_version || 1) < 2" class="mt-4 grid gap-3 md:grid-cols-2">
              <div v-for="artifact in artifactRows(release)" :key="artifact.id || artifact.package_filename" class="legacy-artifact">
                <span class="artifact-family">{{ artifactLabel(artifact.host_family || artifact.target || '') }}</span>
                <strong class="min-w-0 flex-1 truncate text-xs text-gray-700 dark:text-gray-200">{{ artifact.package_filename }}</strong>
                <span class="text-xs text-gray-400">{{ formatBytes(artifact.size_bytes) }}</span>
              </div>
            </div>

            <footer class="mt-4 flex flex-wrap gap-x-6 gap-y-2 border-t border-gray-100 pt-3 text-xs text-gray-500 dark:border-dark-800">
              <span>Runtime 协议 {{ release.runtime_protocol || '-' }}</span>
              <span>Codex {{ release.min_codex_version || '未限制' }}</span>
              <span>Claude {{ release.min_claude_version || '未限制' }}</span>
              <span>{{ formatDate(release.published_at || release.created_at) }}</span>
            </footer>
          </article>
          <p v-if="!loading && releases.length === 0" class="border-y border-gray-200 py-12 text-center text-sm text-gray-500 dark:border-dark-700">尚未创建 Yingzo 发行版本</p>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { Icon } from '@/components/icons'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  createYingzoReleaseDraft, deleteYingzoReleaseArtifact, disableYingzoRelease,
  getYingzoAdminSettings, listYingzoReleases, publishYingzoRelease,
  promoteYingzoRelease, replaceYingzoReleaseArtifact, rollbackYingzoRelease, updateYingzoAdminSettings,
  uploadYingzoReleaseArtifact, type YingzoAdminSettings, type YingzoArtifactUploadInput,
  type YingzoReleaseArtifact, type YingzoReleaseSummary, verifyYingzoReleaseProof,
} from '@/api/yingzo'

type ArtifactSlot = Omit<YingzoArtifactUploadInput, 'file' | 'runtime_protocol'> & { key: string; label: string; filename: (version: string) => string; accept: string }

const artifactSlots: ArtifactSlot[] = [
  { key: 'openai-macos-any', label: 'OpenAI / Codex · macOS', artifact_kind: 'host_package', target: 'openai', os: 'macos', arch: 'any', accept: '.gz,application/gzip', filename: (v) => `yingzo-openai-macos-${v}.tar.gz` },
  { key: 'openai-windows-x64', label: 'OpenAI / Codex · Windows x64', artifact_kind: 'host_package', target: 'openai', os: 'windows', arch: 'x64', accept: '.zip,application/zip', filename: (v) => `yingzo-openai-windows-x64-${v}.zip` },
  { key: 'claude-code-macos-any', label: 'Claude Code · macOS', artifact_kind: 'host_package', target: 'claude-code', os: 'macos', arch: 'any', accept: '.zip,application/zip', filename: (v) => `yingzo-claude-code-macos-${v}.zip` },
  { key: 'claude-code-windows-x64', label: 'Claude Code · Windows x64', artifact_kind: 'host_package', target: 'claude-code', os: 'windows', arch: 'x64', accept: '.zip,application/zip', filename: (v) => `yingzo-claude-code-windows-x64-${v}.zip` },
  { key: 'claude-desktop-any-any', label: 'Claude Desktop / Cowork', artifact_kind: 'host_package', target: 'claude-desktop', os: 'any', arch: 'any', accept: '.mcpb,application/zip', filename: (v) => `yingzo-claude-desktop-${v}.mcpb` },
  { key: 'runtime-macos-arm64', label: 'Runtime · macOS arm64', artifact_kind: 'runtime_installer', target: 'runtime', os: 'macos', arch: 'arm64', accept: '.dmg,application/x-apple-diskimage', filename: (v) => `yingzo-runtime-macos-arm64-${v}.dmg` },
  { key: 'runtime-macos-x64', label: 'Runtime · macOS Intel', artifact_kind: 'runtime_installer', target: 'runtime', os: 'macos', arch: 'x64', accept: '.dmg,application/x-apple-diskimage', filename: (v) => `yingzo-runtime-macos-x64-${v}.dmg` },
  { key: 'runtime-windows-x64', label: 'Runtime · Windows x64', artifact_kind: 'runtime_installer', target: 'runtime', os: 'windows', arch: 'x64', accept: '.exe,application/vnd.microsoft.portable-executable', filename: (v) => `yingzo-runtime-windows-x64-${v}-setup.exe` },
]

const releases = ref<YingzoReleaseSummary[]>([])
const settings = ref<YingzoAdminSettings | null>(null)
const publicOrigin = ref('')
const loading = ref(false)
const savingSettings = ref(false)
const creatingDraft = ref(false)
const uploadingSlot = ref<string | null>(null)
const selectedFiles = reactive<Record<string, File | undefined>>({})
const proofFiles = reactive<Record<string, File | undefined>>({})
const verifyingProof = ref<string | null>(null)
const message = ref('')
const messageKind = ref<'success' | 'error'>('success')
const draftForm = reactive({ version: '0.3.0', channel: 'prerelease' as 'prerelease' | 'stable', runtimeProtocol: 1, minCodex: '0.143.0', minClaude: '2.1.201', notes: '' })

function showMessage(text: string, kind: 'success' | 'error' = 'success') { message.value = text; messageKind.value = kind }
function apiErrorDetail(error: unknown, fallback: string) {
  const nested = (error as { error?: { message?: string; code?: string } })?.error
  return nested?.message || nested?.code || extractApiErrorMessage(error, fallback)
}
function statusLabel(status?: string) { return ({ draft: '草稿', published: '当前发布', superseded: '历史版本', disabled: '已停用' } as Record<string, string>)[status || ''] || status || '未知' }
function channelLabel(channel?: string) { return channel === 'prerelease' ? '预发布' : '稳定版' }
function artifactLabel(value: string) { return ({ openai: 'OpenAI', claude: 'Claude', combined: '旧版双宿主' } as Record<string, string>)[value] || value }
function formatBytes(value: number) { return value >= 1048576 ? `${(value / 1048576).toFixed(1)} MB` : `${Math.max(1, Math.ceil(value / 1024))} KB` }
function formatDate(value?: string) { return value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '-' }
function expectedFilename(slot: ArtifactSlot, version: string) { return slot.filename(version) }
function selectionKey(releaseID: string, slotKey: string) { return `${releaseID}:${slotKey}` }
function selectedArtifactFile(releaseID: string, slotKey: string) { return selectedFiles[selectionKey(releaseID, slotKey)] }
function selectArtifactFile(releaseID: string, slotKey: string, event: Event) { selectedFiles[selectionKey(releaseID, slotKey)] = (event.target as HTMLInputElement).files?.[0] }

function artifactRows(release: YingzoReleaseSummary): YingzoReleaseArtifact[] {
  if (Array.isArray(release.artifact_items)) return release.artifact_items
  const matrix = release.artifact_matrix
  if (Array.isArray(matrix)) return matrix
  const artifacts = release.artifacts
  if (Array.isArray(artifacts)) return artifacts as YingzoReleaseArtifact[]
  return artifacts ? Object.values(artifacts).filter((item): item is YingzoReleaseArtifact => Boolean(item)) : []
}

function artifactReady(artifact?: YingzoReleaseArtifact) {
  if (!artifact) return false
  const validationPassed = artifact.validation_status === undefined
    ? true
    : artifact.validation_status === 'validated'
  return validationPassed && (artifact.signature_status === undefined || artifact.signature_status === 'verified')
}

function artifactForSlot(release: YingzoReleaseSummary, slot: ArtifactSlot): YingzoReleaseArtifact | undefined {
  return artifactRows(release).find((artifact) => artifact.target === slot.target && artifact.os === slot.os && artifact.arch === slot.arch)
}

function canPublish(release: YingzoReleaseSummary) {
  if ((release.distribution_schema_version || 1) < 2) return artifactRows(release).length >= 1
  return Boolean(release.signature) && artifactSlots.every((slot) => artifactReady(artifactForSlot(release, slot)))
}

async function loadAll() {
  loading.value = true
  try {
    const [releaseItems, currentSettings] = await Promise.all([listYingzoReleases(), getYingzoAdminSettings()])
    releases.value = releaseItems
    settings.value = currentSettings
    publicOrigin.value = currentSettings.public_origin
  } catch (error) { showMessage('读取 Yingzo 发行配置失败。', 'error'); console.error(error) }
  finally { loading.value = false }
}

async function saveSettings() {
  savingSettings.value = true
  try { settings.value = await updateYingzoAdminSettings(publicOrigin.value); publicOrigin.value = settings.value.public_origin; showMessage('通信域名已保存。') }
  catch (error) { showMessage('域名无效或保存失败。', 'error'); console.error(error) }
  finally { savingSettings.value = false }
}

async function createDraft() {
  creatingDraft.value = true
  try {
    const created = await createYingzoReleaseDraft({
      version: draftForm.version,
      channel: draftForm.channel,
      distribution_schema_version: 2,
      runtime_protocol: draftForm.runtimeProtocol,
      compatibility: { platforms: ['macos-arm64', 'macos-x64', 'windows-x64'], artifact_count: 8 },
      min_codex_version: draftForm.minCodex,
      min_claude_version: draftForm.minClaude,
      release_notes: draftForm.notes,
    })
    showMessage(`版本 ${created.version} 的 ${channelLabel(created.channel)}草稿已创建，请上传八个产物。`)
    draftForm.notes = ''
    await loadAll()
  } catch (error) { showMessage(`创建草稿失败：${apiErrorDetail(error, '请检查版本是否重复')}`, 'error'); console.error(error) }
  finally { creatingDraft.value = false }
}

async function uploadArtifact(release: YingzoReleaseSummary, slot: ArtifactSlot) {
  if (!release.id) return
  const file = selectedArtifactFile(release.id, slot.key)
  if (!file) return
  const key = selectionKey(release.id, slot.key)
  uploadingSlot.value = key
  const input: YingzoArtifactUploadInput = {
    ...slot,
    file,
    runtime_protocol: release.runtime_protocol || 1,
  }
  try {
    const existing = artifactForSlot(release, slot)
    if (existing?.id) await replaceYingzoReleaseArtifact(release.id, existing.id, input)
    else await uploadYingzoReleaseArtifact(release.id, input)
    delete selectedFiles[key]
    showMessage(`${slot.label} 已上传并通过内部校验。`)
    await loadAll()
  } catch (error) { showMessage(`上传失败：${apiErrorDetail(error, '请检查文件名、格式和签名状态')}`, 'error'); console.error(error) }
  finally { uploadingSlot.value = null }
}

function selectProofFile(releaseID: string, event: Event) {
  proofFiles[releaseID] = (event.target as HTMLInputElement).files?.[0]
}

function proofReady(releaseID: string) {
  return Boolean(proofFiles[releaseID])
}

async function submitProof(release: YingzoReleaseSummary) {
  if (!release.id || !proofReady(release.id)) return
  verifyingProof.value = release.id
  try {
    const proofFile = proofFiles[release.id]!
    const proofText = await readFileText(proofFile)
    const proof = JSON.parse(proofText) as { algorithm?: string; key_id?: string; manifest_base64?: string; signature_base64?: string }
    if (proof.algorithm !== 'Ed25519' || !proof.key_id || !proof.manifest_base64 || !proof.signature_base64) {
      throw new Error('proof envelope must contain algorithm, key_id, manifest_base64, and signature_base64')
    }
    await verifyYingzoReleaseProof(release.id, {
      algorithm: 'Ed25519',
      key_id: proof.key_id.trim(),
      manifest_base64: proof.manifest_base64,
      signature_base64: proof.signature_base64,
    })
    showMessage('发行 manifest、八项哈希与 Ed25519 detached signature 已由服务端验证。')
    await loadAll()
  } catch (error) {
    showMessage(`发行证明验证失败：${apiErrorDetail(error, '请核对 key_id、manifest 原始字节和 detached signature')}`, 'error')
    console.error(error)
  } finally {
    verifyingProof.value = null
  }
}

async function readFileText(file: File): Promise<string> {
  if (typeof file.text === 'function') return file.text()
  if (typeof file.arrayBuffer === 'function') return new TextDecoder().decode(await file.arrayBuffer())
  return await new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(reader.error || new Error('proof file could not be read'))
    reader.readAsText(file)
  })
}

async function removeArtifact(release: YingzoReleaseSummary, artifact: YingzoReleaseArtifact) {
  if (!release.id || !artifact.id || !window.confirm(`删除 ${artifact.package_filename}？`)) return
  try { await deleteYingzoReleaseArtifact(release.id, artifact.id); showMessage('产物已从草稿删除。'); await loadAll() }
  catch (error) { showMessage('删除产物失败。', 'error'); console.error(error) }
}

async function publish(release: YingzoReleaseSummary) { if (!release.id) return; try { await publishYingzoRelease(release.id); showMessage(`版本 ${release.version} 已发布到${channelLabel(release.channel)}通道。`); await loadAll() } catch (error) { showMessage(`发布失败：${apiErrorDetail(error, '请确认八项产物完整且协议一致')}`, 'error'); console.error(error) } }
async function promote(release: YingzoReleaseSummary) { if (!release.id || !window.confirm(`确认将 ${release.version} 的同一组已签名二进制提升为稳定版？`)) return; try { await promoteYingzoRelease(release.id); showMessage(`版本 ${release.version} 已提升为稳定版，没有重新构建或替换二进制。`); await loadAll() } catch (error) { showMessage(`提升失败：${apiErrorDetail(error, '请确认八项文件仍完整可用')}`, 'error'); console.error(error) } }
async function rollback(release: YingzoReleaseSummary) { if (!release.id || !window.confirm(`确认将${channelLabel(release.channel)}回滚到 ${release.version}？`)) return; try { await rollbackYingzoRelease(release.id); showMessage(`已回滚到 ${release.version}。`); await loadAll() } catch (error) { showMessage('回滚失败。', 'error'); console.error(error) } }
async function disable(release: YingzoReleaseSummary) { if (!release.id || !window.confirm(`确认停用 ${release.version}？`)) return; try { await disableYingzoRelease(release.id); showMessage(`版本 ${release.version} 已停用。`); await loadAll() } catch (error) { showMessage('停用失败。', 'error'); console.error(error) } }

onMounted(loadAll)
</script>

<style scoped>
.yingzo-admin code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
.field-label { font-size: 12px; font-weight: 600; color: #666; }
.release-panel { border: 1px solid #dedede; border-radius: 6px; padding: 20px; }
.artifact-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); gap: 12px; }
.artifact-slot { min-width: 0; border-top: 2px solid #d5d5d5; padding-top: 12px; }
.legacy-artifact { display: flex; align-items: center; gap: 8px; min-width: 0; }
.status-mark, .channel-mark, .verified-mark, .unverified-mark, .missing-mark { display: inline-flex; align-items: center; border-radius: 4px; padding: 3px 8px; font-size: 12px; font-weight: 650; }
.status-draft { background: #f1f1f1; color: #555; }
.status-published { background: #e7f6ed; color: #18733c; }
.status-superseded { background: #fff0ed; color: #a33f30; }
.status-disabled { background: #fce8e8; color: #a32121; }
.channel-mark { background: #edf2f8; color: #35536f; }
.verified-mark { background: #e7f6ed; color: #18733c; padding: 2px 6px; font-size: 11px; }
.unverified-mark { background: #fff6dc; color: #8b6511; padding: 2px 6px; font-size: 11px; }
.missing-mark { background: #fff0ed; color: #a33f30; padding: 2px 6px; font-size: 11px; }
.proof-panel { margin-top: 18px; border-top: 2px solid #35536f; padding-top: 14px; }
.proof-inputs { margin-top: 10px; display: grid; grid-template-columns: minmax(160px, .7fr) repeat(2, minmax(220px, 1fr)) auto; gap: 8px; }
@media (max-width: 1100px) { .proof-inputs { grid-template-columns: 1fr; } }
.artifact-family { min-width: 64px; border-radius: 3px; background: #f2f2f2; padding: 2px 6px; color: #555; font-weight: 700; text-align: center; }
:global(.dark) .field-label { color: #aaa; }
:global(.dark) .release-panel { border-color: #3c3c3c; }
:global(.dark) .artifact-slot { border-color: #555; }
</style>
