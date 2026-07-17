<template>
  <AppLayout>
    <div class="yingzo-admin space-y-8">
      <header class="flex flex-col justify-between gap-4 border-b border-gray-200 pb-6 dark:border-dark-700 md:flex-row md:items-end">
        <div>
          <h1 class="text-2xl font-semibold text-gray-950 dark:text-white">Yingzo（影作）发行管理</h1>
          <p class="mt-2 max-w-3xl text-sm leading-6 text-gray-500 dark:text-gray-400">管理公开域名、OpenAI/Claude 双宿主安装包和当前发布版本。安装包保存在服务器数据目录，不进入公开 Git 仓库。</p>
        </div>
        <router-link to="/yingzo" class="btn btn-secondary btn-sm">查看产品页</router-link>
      </header>

      <section class="grid gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(420px,.8fr)]">
        <div class="border-t-2 border-gray-900 pt-5 dark:border-white">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">通信域名</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">为空时从当前请求域名推导；生产部署建议保存为 <code>https://api-key.cc</code>。</p>
          <div class="mt-5 flex flex-col gap-3 sm:flex-row">
            <input v-model="publicOrigin" class="input min-w-0 flex-1" placeholder="https://api-key.cc" autocomplete="off" />
            <button type="button" class="btn btn-primary" :disabled="savingSettings" @click="saveSettings">
              {{ savingSettings ? '保存中' : '保存域名' }}
            </button>
          </div>
          <dl class="mt-4 grid gap-2 text-xs text-gray-500 sm:grid-cols-2 dark:text-gray-400">
            <div><dt class="font-medium text-gray-700 dark:text-gray-300">当前生效</dt><dd class="mt-1 break-all">{{ settings?.effective_origin || '读取中' }}</dd></div>
            <div><dt class="font-medium text-gray-700 dark:text-gray-300">发行存储</dt><dd class="mt-1 break-all">{{ settings?.release_storage || '读取中' }}</dd></div>
          </dl>
        </div>

        <form class="border-t-2 border-[#df5b48] pt-5" @submit.prevent="uploadRelease">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">上传新版本</h2>
          <div class="mt-5 grid gap-3 sm:grid-cols-2">
            <label class="text-xs font-medium text-gray-600 dark:text-gray-400">版本
              <input v-model="uploadForm.version" required class="input mt-1 w-full" placeholder="0.1.3" />
            </label>
            <label class="text-xs font-medium text-gray-600 dark:text-gray-400">OpenAI 安装包
              <input ref="openAIFileInput" required type="file" accept=".gz,application/gzip" class="input mt-1 w-full py-1.5" @change="onFileChange('openai', $event)" />
              <span class="mt-1 block font-normal text-gray-400">yingzo-openai-版本.tar.gz</span>
            </label>
            <label class="text-xs font-medium text-gray-600 dark:text-gray-400">Claude 安装包
              <input ref="claudeFileInput" required type="file" accept=".gz,application/gzip" class="input mt-1 w-full py-1.5" @change="onFileChange('claude', $event)" />
              <span class="mt-1 block font-normal text-gray-400">yingzo-claude-版本.tar.gz</span>
            </label>
            <label class="text-xs font-medium text-gray-600 dark:text-gray-400">最低 Codex 版本
              <input v-model="uploadForm.minCodex" class="input mt-1 w-full" placeholder="0.143.0" />
            </label>
            <label class="text-xs font-medium text-gray-600 dark:text-gray-400">最低 Claude Code 版本
              <input v-model="uploadForm.minClaude" class="input mt-1 w-full" placeholder="2.1.201" />
            </label>
            <label class="text-xs font-medium text-gray-600 sm:col-span-2 dark:text-gray-400">发行签名（可选）
              <input v-model="uploadForm.signature" class="input mt-1 w-full font-mono" />
            </label>
            <label class="text-xs font-medium text-gray-600 sm:col-span-2 dark:text-gray-400">更新说明
              <textarea v-model="uploadForm.notes" class="input mt-1 min-h-20 w-full py-2" />
            </label>
          </div>
          <button type="submit" class="btn btn-primary mt-4 inline-flex items-center gap-2" :disabled="uploading">
            <Icon name="upload" size="sm" />{{ uploading ? '上传并校验中' : '上传草稿版本' }}
          </button>
        </form>
      </section>

      <p v-if="message" class="border-l-2 px-3 py-2 text-sm" :class="messageKind === 'error' ? 'border-red-500 bg-red-50 text-red-700 dark:bg-red-950/30 dark:text-red-300' : 'border-emerald-500 bg-emerald-50 text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-300'">{{ message }}</p>

      <section>
        <div class="mb-4 flex items-center justify-between">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">发行记录</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">同一版本的两个宿主包全部通过内部完整性校验后才能整体发布或回滚。</p>
          </div>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="loadAll">
            <Icon name="refresh" size="sm" />
          </button>
        </div>
        <div class="overflow-x-auto border-y border-gray-200 dark:border-dark-700">
          <table class="w-full min-w-[980px] text-left text-sm">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900 dark:text-gray-400">
              <tr><th class="px-4 py-3">版本</th><th class="px-4 py-3">状态</th><th class="px-4 py-3">宿主安装包</th><th class="px-4 py-3">兼容性</th><th class="px-4 py-3">更新时间</th><th class="px-4 py-3 text-right">操作</th></tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
              <tr v-for="release in releases" :key="release.id" class="align-top">
                <td class="px-4 py-4 font-semibold text-gray-900 dark:text-white">{{ release.version }}</td>
                <td class="px-4 py-4"><span class="status-mark" :class="`status-${release.status}`">{{ statusLabel(release.status) }}</span></td>
                <td class="min-w-[320px] px-4 py-4">
                  <div v-for="artifact in artifactRows(release)" :key="`${release.id}-${artifact.host_family}`" class="mb-3 last:mb-0">
                    <div class="flex items-center gap-2 text-xs"><span class="artifact-family">{{ artifactLabel(artifact.host_family) }}</span><strong class="truncate text-gray-700 dark:text-gray-200" :title="artifact.package_filename">{{ artifact.package_filename }}</strong><span class="text-gray-400">{{ formatBytes(artifact.size_bytes) }}</span></div>
                    <code v-if="artifact.sha256" class="mt-1 block max-w-[420px] truncate text-[11px] text-gray-400" :title="artifact.sha256">内部校验 {{ artifact.sha256 }}</code>
                  </div>
                </td>
                <td class="px-4 py-4 text-xs leading-5 text-gray-500">Codex {{ release.min_codex_version || '未限制' }}<br />Claude {{ release.min_claude_version || '未限制' }}</td>
                <td class="px-4 py-4 text-xs text-gray-500">{{ formatDate(release.published_at || release.created_at) }}</td>
                <td class="px-4 py-4"><div class="flex justify-end gap-2">
                  <button v-if="release.status === 'draft'" type="button" class="btn btn-primary btn-xs" @click="publish(release)">发布</button>
                  <button v-if="release.status === 'superseded'" type="button" class="btn btn-secondary btn-xs" @click="rollback(release)">回滚至此版本</button>
                  <button v-if="release.status !== 'disabled'" type="button" class="btn btn-danger btn-xs" @click="disable(release)">停用</button>
                </div></td>
              </tr>
              <tr v-if="!loading && releases.length === 0"><td colspan="6" class="px-4 py-12 text-center text-gray-500">尚未上传 Yingzo 安装包</td></tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { Icon } from '@/components/icons'
import {
  disableYingzoRelease, getYingzoAdminSettings, listYingzoReleases, publishYingzoRelease,
  rollbackYingzoRelease, updateYingzoAdminSettings, uploadYingzoRelease,
  type YingzoAdminSettings, type YingzoReleaseArtifact, type YingzoReleaseSummary,
} from '@/api/yingzo'

const releases = ref<YingzoReleaseSummary[]>([])
const settings = ref<YingzoAdminSettings | null>(null)
const publicOrigin = ref('')
const loading = ref(false)
const savingSettings = ref(false)
const uploading = ref(false)
const selectedOpenAIFile = ref<File | null>(null)
const selectedClaudeFile = ref<File | null>(null)
const openAIFileInput = ref<HTMLInputElement | null>(null)
const claudeFileInput = ref<HTMLInputElement | null>(null)
const message = ref('')
const messageKind = ref<'success' | 'error'>('success')
const uploadForm = reactive({ version: '', minCodex: '0.143.0', minClaude: '2.1.201', signature: '', notes: '' })

function showMessage(text: string, kind: 'success' | 'error' = 'success') { message.value = text; messageKind.value = kind }
function onFileChange(family: 'openai' | 'claude', event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0] || null
  if (family === 'openai') selectedOpenAIFile.value = file
  else selectedClaudeFile.value = file
}
function statusLabel(status?: string) { return ({ draft: '草稿', published: '当前发布', superseded: '历史版本', disabled: '已停用' } as Record<string, string>)[status || ''] || status }
function artifactLabel(family: string) { return ({ openai: 'OpenAI', claude: 'Claude', combined: '旧版双宿主' } as Record<string, string>)[family] || family }
function artifactRows(release: YingzoReleaseSummary): YingzoReleaseArtifact[] {
  return ['openai', 'claude', 'combined'].flatMap((family) => {
    const artifact = release.artifacts?.[family as keyof YingzoReleaseSummary['artifacts']]
    return artifact ? [artifact] : []
  })
}
function formatBytes(value: number) { return value >= 1048576 ? `${(value / 1048576).toFixed(1)} MB` : `${Math.ceil(value / 1024)} KB` }
function formatDate(value?: string) { return value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '-' }

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

async function uploadRelease() {
  if (!selectedOpenAIFile.value || !selectedClaudeFile.value) { showMessage('请选择同一版本的 OpenAI 和 Claude 两个 .tar.gz 安装包。', 'error'); return }
  uploading.value = true
  const form = new FormData()
  form.set('openai_package', selectedOpenAIFile.value)
  form.set('claude_package', selectedClaudeFile.value)
  form.set('version', uploadForm.version)
  form.set('min_codex_version', uploadForm.minCodex)
  form.set('min_claude_version', uploadForm.minClaude)
  form.set('signature', uploadForm.signature)
  form.set('release_notes', uploadForm.notes)
  try {
    await uploadYingzoRelease(form)
    showMessage(`版本 ${uploadForm.version} 的双宿主安装包已上传并通过内部完整性校验，等待发布。`)
    uploadForm.version = ''; uploadForm.signature = ''; uploadForm.notes = ''; selectedOpenAIFile.value = null; selectedClaudeFile.value = null
    if (openAIFileInput.value) openAIFileInput.value.value = ''
    if (claudeFileInput.value) claudeFileInput.value.value = ''
    await loadAll()
  } catch (error) {
    const response = (error as { response?: { data?: { error?: { message?: string; code?: string } } } }).response
    const detail = response?.data?.error?.message || response?.data?.error?.code
    showMessage(detail ? `上传失败：${detail}` : '上传失败。请检查版本是否重复以及安装包文件名和内容。', 'error')
    console.error(error)
  }
  finally { uploading.value = false }
}

async function publish(release: YingzoReleaseSummary) { if (!release.id) return; try { await publishYingzoRelease(release.id); showMessage(`版本 ${release.version} 已发布。`); await loadAll() } catch (error) { showMessage('发布失败。', 'error'); console.error(error) } }
async function rollback(release: YingzoReleaseSummary) { if (!release.id || !window.confirm(`确认回滚到 ${release.version}？`)) return; try { await rollbackYingzoRelease(release.id); showMessage(`已回滚到 ${release.version}。`); await loadAll() } catch (error) { showMessage('回滚失败。', 'error'); console.error(error) } }
async function disable(release: YingzoReleaseSummary) { if (!release.id || !window.confirm(`确认停用 ${release.version}？`)) return; try { await disableYingzoRelease(release.id); showMessage(`版本 ${release.version} 已停用。`); await loadAll() } catch (error) { showMessage('停用失败。', 'error'); console.error(error) } }

onMounted(loadAll)
</script>

<style scoped>
.yingzo-admin code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
.status-mark { display: inline-flex; align-items: center; border-radius: 4px; padding: 3px 8px; font-size: 12px; font-weight: 650; }
.status-draft { background: #f1f1f1; color: #555; }
.status-published { background: #e7f6ed; color: #18733c; }
.status-superseded { background: #fff0ed; color: #a33f30; }
.status-disabled { background: #fce8e8; color: #a32121; }
.artifact-family { min-width: 64px; border-radius: 3px; background: #f2f2f2; padding: 2px 6px; color: #555; font-weight: 700; text-align: center; }
</style>
