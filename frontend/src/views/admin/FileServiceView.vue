<template>
  <AppLayout>
    <div class="space-y-8">
      <header class="border-b border-gray-200 pb-6 dark:border-dark-700">
        <div class="flex flex-col justify-between gap-4 md:flex-row md:items-end">
          <div>
            <p class="text-xs font-semibold uppercase text-primary-600 dark:text-primary-400">Shared infrastructure</p>
            <h1 class="mt-2 text-2xl font-semibold text-gray-950 dark:text-white">文件服务</h1>
            <p class="mt-2 max-w-3xl text-sm leading-6 text-gray-500 dark:text-gray-400">
              为图片、视频、音频和未来模型任务提供统一的临时公网文件、存储配额与生命周期管理。
            </p>
          </div>
          <button type="button" class="btn btn-secondary btn-sm inline-flex items-center gap-2" :disabled="loading" @click="loadSettings">
            <Icon name="refresh" size="sm" />{{ loading ? '读取中' : '刷新状态' }}
          </button>
        </div>
      </header>

      <section aria-label="文件服务状态" class="grid border-y border-gray-200 dark:border-dark-700 sm:grid-cols-2 xl:grid-cols-4">
        <div class="px-0 py-4 sm:px-4 sm:first:pl-0">
          <p class="text-xs text-gray-500 dark:text-gray-400">当前后端</p>
          <div class="mt-2 flex items-center gap-2">
            <span class="h-2 w-2 rounded-full" :class="form.backend === 's3' ? 'bg-sky-500' : 'bg-emerald-500'" />
            <strong class="text-sm text-gray-900 dark:text-white">{{ form.backend === 's3' ? 'S3 / MinIO' : '服务器本地' }}</strong>
          </div>
          <p class="mt-1 text-xs text-gray-400">配置来源：{{ sourceLabel }}</p>
        </div>
        <div class="border-gray-200 px-0 py-4 dark:border-dark-700 sm:border-l sm:px-4">
          <p class="text-xs text-gray-500 dark:text-gray-400">有效文件</p>
          <p class="mt-2 text-xl font-semibold tabular-nums text-gray-900 dark:text-white">{{ settings?.usage.active_files ?? 0 }}</p>
          <p class="mt-1 text-xs text-gray-400">本地 {{ settings?.usage.local_files ?? 0 }} · S3 {{ settings?.usage.s3_files ?? 0 }}</p>
        </div>
        <div class="border-gray-200 px-0 py-4 dark:border-dark-700 xl:border-l xl:px-4">
          <p class="text-xs text-gray-500 dark:text-gray-400">占用空间</p>
          <p class="mt-2 text-xl font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatBytes(settings?.usage.active_bytes ?? 0) }}</p>
          <p class="mt-1 text-xs text-gray-400">一小时内过期 {{ settings?.usage.expiring_within_1_hour ?? 0 }} 个</p>
        </div>
        <div class="border-gray-200 px-0 py-4 dark:border-dark-700 sm:border-l sm:px-4">
          <p class="text-xs text-gray-500 dark:text-gray-400">公网基址</p>
          <p class="mt-2 break-all font-mono text-sm font-medium text-gray-900 dark:text-white">{{ settings?.effective_public_base_url || '读取中' }}</p>
          <p class="mt-1 text-xs text-gray-400">无查询参数的稳定媒体路径</p>
        </div>
      </section>

      <form class="space-y-8" @submit.prevent="saveSettings">
        <section class="grid gap-6 xl:grid-cols-[280px_minmax(0,1fr)]">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">存储后端</h2>
            <p class="mt-2 text-sm leading-6 text-gray-500 dark:text-gray-400">选择新上传文件的落点。切换不会删除已登记文件；修改 S3 Bucket 或凭证前应等待旧文件过期。</p>
          </div>
          <div class="border-t-2 border-gray-900 pt-5 dark:border-white">
            <div class="inline-flex rounded-md border border-gray-200 bg-gray-50 p-1 dark:border-dark-700 dark:bg-dark-900" role="group" aria-label="存储后端">
              <button type="button" class="min-h-10 px-4 text-sm font-medium transition-colors" :class="form.backend === 'local' ? 'bg-white text-gray-950 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 dark:text-gray-400'" @click="form.backend = 'local'">
                服务器本地
              </button>
              <button type="button" class="min-h-10 px-4 text-sm font-medium transition-colors" :class="form.backend === 's3' ? 'bg-white text-gray-950 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 dark:text-gray-400'" @click="form.backend = 's3'">
                S3 / MinIO
              </button>
            </div>

            <div v-if="form.backend === 'local'" class="mt-5 border-l-2 border-emerald-500 bg-emerald-50 px-4 py-3 dark:bg-emerald-950/20">
              <p class="text-sm font-medium text-emerald-800 dark:text-emerald-300">本地存储已就绪</p>
              <p class="mt-1 break-all font-mono text-xs text-emerald-700/80 dark:text-emerald-400">{{ settings?.local_path || '读取中' }}</p>
            </div>

            <div v-else class="mt-5 grid gap-4 md:grid-cols-2">
              <label class="field-label md:col-span-2" for="file-service-endpoint">Endpoint
                <input id="file-service-endpoint" v-model.trim="form.s3.endpoint" required class="input mt-1.5 w-full" placeholder="https://<account>.r2.cloudflarestorage.com" autocomplete="off" />
              </label>
              <label class="field-label" for="file-service-region">Region
                <input id="file-service-region" v-model.trim="form.s3.region" required class="input mt-1.5 w-full" placeholder="auto" autocomplete="off" />
              </label>
              <label class="field-label" for="file-service-bucket">Bucket
                <input id="file-service-bucket" v-model.trim="form.s3.bucket" required class="input mt-1.5 w-full" autocomplete="off" />
              </label>
              <label class="field-label" for="file-service-access-key">Access Key ID
                <input id="file-service-access-key" v-model.trim="form.s3.access_key_id" required class="input mt-1.5 w-full" autocomplete="off" />
              </label>
              <label class="field-label" for="file-service-secret">Secret Access Key
                <span class="relative mt-1.5 block">
                  <input id="file-service-secret" v-model="form.s3.secret_access_key" :type="showSecret ? 'text' : 'password'" :required="!secretConfigured" class="input w-full pr-11" :placeholder="secretConfigured ? '已加密保存，留空保持不变' : '请输入密钥'" autocomplete="new-password" />
                  <button type="button" class="absolute inset-y-0 right-0 flex w-11 items-center justify-center text-gray-400 hover:text-gray-700 dark:hover:text-gray-200" :aria-label="showSecret ? '隐藏密钥' : '显示密钥'" @click="showSecret = !showSecret">
                    <Icon :name="showSecret ? 'eyeOff' : 'eye'" size="sm" />
                  </button>
                </span>
              </label>
              <label class="field-label md:col-span-2" for="file-service-prefix">对象前缀
                <input id="file-service-prefix" v-model.trim="form.s3.prefix" required class="input mt-1.5 w-full font-mono" placeholder="model-assets/" autocomplete="off" />
              </label>
              <label class="flex min-h-11 items-center gap-3 text-sm text-gray-700 dark:text-gray-300 md:col-span-2">
                <input v-model="form.s3.force_path_style" type="checkbox" class="h-4 w-4" />
                <span>启用 Path Style（MinIO 通常需要）</span>
              </label>
            </div>
          </div>
        </section>

        <section class="grid gap-6 xl:grid-cols-[280px_minmax(0,1fr)]">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">访问与生命周期</h2>
            <p class="mt-2 text-sm leading-6 text-gray-500 dark:text-gray-400">公网基址只包含协议和域名。文件身份仍由数据库 ID 管理，24 小时配额按 API Key 与用户滚动计算。</p>
          </div>
          <div class="grid gap-4 border-t-2 border-primary-500 pt-5 md:grid-cols-2">
            <label class="field-label md:col-span-2" for="file-service-public-base">公网基址
              <input id="file-service-public-base" v-model.trim="form.public_base_url" class="input mt-1.5 w-full" placeholder="https://api-key.cc" autocomplete="url" />
              <span class="mt-1.5 block text-xs font-normal text-gray-400">示例：{{ previewPublicURL }}</span>
            </label>
            <label class="field-label" for="file-service-retention">保留时间（小时）
              <input id="file-service-retention" v-model.number="form.retention_hours" type="number" min="1" max="720" required class="input mt-1.5 w-full" />
            </label>
            <label class="field-label" for="file-service-max-count">每个密钥 24 小时文件数
              <input id="file-service-max-count" v-model.number="form.daily_max_count" type="number" min="1" max="1000000" required class="input mt-1.5 w-full" />
            </label>
            <label class="field-label md:col-span-2" for="file-service-max-bytes">每个密钥 24 小时总字节数
              <input id="file-service-max-bytes" v-model.number="form.daily_max_bytes" type="number" min="1" required class="input mt-1.5 w-full font-mono" />
              <span class="mt-1.5 block text-xs font-normal text-gray-400">当前输入：{{ formatBytes(form.daily_max_bytes) }}</span>
            </label>
            <div class="md:col-span-2 grid gap-2 border-y border-gray-200 py-3 text-xs text-gray-500 dark:border-dark-700 dark:text-gray-400 sm:grid-cols-3">
              <span>图片单文件 30 MB</span><span>视频单文件 200 MB</span><span>音频单文件 15 MB</span>
            </div>
          </div>
        </section>

        <p v-if="message" :role="messageKind === 'error' ? 'alert' : 'status'" class="border-l-2 px-3 py-2 text-sm" :class="messageKind === 'error' ? 'border-red-500 bg-red-50 text-red-700 dark:bg-red-950/30 dark:text-red-300' : 'border-emerald-500 bg-emerald-50 text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-300'">
          {{ message }}
        </p>

        <div class="flex flex-wrap justify-end gap-3 border-t border-gray-200 pt-5 dark:border-dark-700">
          <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="testing || saving" @click="testConnection">
            <Icon name="beaker" size="sm" />{{ testing ? '测试中' : '测试当前配置' }}
          </button>
          <button type="submit" class="btn btn-primary inline-flex items-center gap-2" :disabled="saving || loading">
            <Icon name="check" size="sm" />{{ saving ? '保存并切换中' : '保存并应用' }}
          </button>
        </div>
      </form>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { Icon } from '@/components/icons'
import {
  getFileStorageSettings,
  testFileStorageSettings,
  updateFileStorageSettings,
  type FileStorageSettings,
  type FileStorageUpdate,
} from '@/api/fileService'

const emptyForm = (): FileStorageUpdate => ({
  schema_version: 1,
  backend: 'local',
  public_base_url: '',
  retention_hours: 24,
  daily_max_count: 100,
  daily_max_bytes: 2 * 1024 * 1024 * 1024,
  s3: {
    endpoint: '',
    region: 'auto',
    bucket: '',
    access_key_id: '',
    secret_access_key: '',
    prefix: 'model-assets/',
    force_path_style: false,
  },
})

const settings = ref<FileStorageSettings | null>(null)
const form = reactive<FileStorageUpdate>(emptyForm())
const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const showSecret = ref(false)
const secretConfigured = ref(false)
const message = ref('')
const messageKind = ref<'success' | 'error'>('success')

const sourceLabel = computed(() => ({ database: '管理后台', environment: '部署兼容配置', default: '系统默认' })[settings.value?.source || 'default'])
const previewPublicURL = computed(() => `${form.public_base_url || settings.value?.effective_public_base_url || 'https://api-key.cc'}/media/<asset-id>/asset.png`)

function applySettings(next: FileStorageSettings) {
  settings.value = next
  secretConfigured.value = next.secret_access_key_configured
  Object.assign(form, {
    schema_version: next.schema_version,
    backend: next.backend,
    public_base_url: next.public_base_url,
    retention_hours: next.retention_hours,
    daily_max_count: next.daily_max_count,
    daily_max_bytes: next.daily_max_bytes,
    s3: { ...next.s3, secret_access_key: '' },
  })
}

function showMessage(text: string, kind: 'success' | 'error' = 'success') {
  message.value = text
  messageKind.value = kind
}

function errorMessage(error: unknown, fallback: string) {
  if (error && typeof error === 'object' && 'message' in error && typeof error.message === 'string') return error.message
  return fallback
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  return `${(value / Math.pow(1024, index)).toFixed(index === 0 ? 0 : 1)} ${units[index]}`
}

async function loadSettings() {
  loading.value = true
  try {
    applySettings(await getFileStorageSettings())
  } catch (error) {
    showMessage(errorMessage(error, '读取文件服务配置失败。'), 'error')
  } finally {
    loading.value = false
  }
}

async function testConnection() {
  testing.value = true
  try {
    const result = await testFileStorageSettings({ ...form, s3: { ...form.s3 } })
    showMessage(result.ok ? `${form.backend === 's3' ? 'S3 / MinIO' : '本地目录'}连接测试通过。` : `连接测试失败：${result.message}`, result.ok ? 'success' : 'error')
  } catch (error) {
    showMessage(`连接测试失败：${errorMessage(error, '请求失败')}`, 'error')
  } finally {
    testing.value = false
  }
}

async function saveSettings() {
  saving.value = true
  try {
    applySettings(await updateFileStorageSettings({ ...form, s3: { ...form.s3 } }))
    showMessage('文件服务配置已加密保存并立即应用。')
  } catch (error) {
    showMessage(`保存失败：${errorMessage(error, '请检查配置与连接状态')}`, 'error')
  } finally {
    saving.value = false
  }
}

onMounted(loadSettings)
</script>

<style scoped>
.field-label { display: block; font-size: 0.75rem; font-weight: 600; line-height: 1.25rem; color: rgb(75 85 99); }
:global(.dark) .field-label { color: rgb(156 163 175); }
</style>
