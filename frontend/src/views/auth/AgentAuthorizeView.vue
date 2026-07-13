<template>
  <AuthLayout>
    <div class="space-y-6">
      <div class="text-center">
        <div class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-400">
          <Icon :name="approved ? 'checkCircle' : 'shield'" size="lg" />
        </div>
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ approved ? t('agentAuthorization.approvedTitle') : t('agentAuthorization.title') }}
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ approved ? t('agentAuthorization.approvedDescription') : t('agentAuthorization.description') }}
        </p>
      </div>

      <div v-if="approved" class="rounded-lg border border-green-200 bg-green-50 p-4 text-sm text-green-800 dark:border-green-800/60 dark:bg-green-900/20 dark:text-green-200">
        {{ t('agentAuthorization.returnToYingzo') }}
      </div>

      <form v-else class="space-y-5" @submit.prevent="loadAuthorization">
        <div>
          <label for="user-code" class="input-label">{{ t('agentAuthorization.codeLabel') }}</label>
          <input
            id="user-code"
            v-model="userCode"
            class="input text-center font-mono text-lg uppercase"
            type="text"
            autocomplete="one-time-code"
            maxlength="16"
            :disabled="loading || approving"
            :placeholder="t('agentAuthorization.codePlaceholder')"
            @input="normalizeCode"
          />
        </div>

        <div v-if="errorMessage" class="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-800/60 dark:bg-red-900/20 dark:text-red-300">
          {{ errorMessage }}
        </div>

        <template v-if="authorization">
          <dl class="divide-y divide-gray-100 rounded-lg border border-gray-200 bg-gray-50 px-4 dark:divide-dark-700 dark:border-dark-700 dark:bg-dark-900/60">
            <div class="flex items-center justify-between gap-4 py-3">
              <dt class="text-sm text-gray-500 dark:text-dark-400">{{ t('agentAuthorization.application') }}</dt>
              <dd class="text-sm font-semibold text-gray-900 dark:text-white">Yingzo</dd>
            </div>
            <div class="flex items-center justify-between gap-4 py-3">
              <dt class="text-sm text-gray-500 dark:text-dark-400">{{ t('agentAuthorization.installation') }}</dt>
              <dd class="min-w-0 truncate text-sm font-medium text-gray-900 dark:text-white">{{ authorization.installation_name }}</dd>
            </div>
            <div class="flex items-center justify-between gap-4 py-3">
              <dt class="text-sm text-gray-500 dark:text-dark-400">{{ t('agentAuthorization.expires') }}</dt>
              <dd class="text-sm font-medium text-gray-900 dark:text-white">{{ formattedExpiry }}</dd>
            </div>
          </dl>

          <div class="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-800/60 dark:bg-amber-900/20 dark:text-amber-200">
            {{ t('agentAuthorization.permissionNotice') }}
          </div>

          <button type="button" class="btn btn-primary w-full" :disabled="approving" @click="approve">
            <span v-if="approving" class="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-white/40 border-t-white" aria-hidden="true"></span>
            <Icon v-else name="shield" size="md" class="mr-2" />
            {{ approving ? t('agentAuthorization.approving') : t('agentAuthorization.approve') }}
          </button>
        </template>

        <button v-else type="submit" class="btn btn-primary w-full" :disabled="loading || userCode.length < 6">
          <span v-if="loading" class="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-white/40 border-t-white" aria-hidden="true"></span>
          <Icon v-else name="search" size="md" class="mr-2" />
          {{ loading ? t('agentAuthorization.checking') : t('agentAuthorization.continue') }}
        </button>
      </form>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { agentAPI, type AgentDeviceAuthorization } from '@/api'
import { AuthLayout } from '@/components/layout'
import Icon from '@/components/icons/Icon.vue'

const route = useRoute()
const { t, locale } = useI18n()
const userCode = ref('')
const authorization = ref<AgentDeviceAuthorization | null>(null)
const loading = ref(false)
const approving = ref(false)
const approved = ref(false)
const errorMessage = ref('')

const formattedExpiry = computed(() => {
  if (!authorization.value) return ''
  return new Intl.DateTimeFormat(locale.value, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(authorization.value.expires_at))
})

function normalizeCode(): void {
  userCode.value = userCode.value.toUpperCase().replace(/[^A-Z0-9_-]/g, '')
  authorization.value = null
  errorMessage.value = ''
}

function authorizationError(error: unknown): string {
  const responseError = error as { response?: { data?: { error?: { code?: string } } } }
  if (responseError.response?.data?.error?.code === 'authorization_not_found') {
    return t('agentAuthorization.notFound')
  }
  return t('agentAuthorization.requestFailed')
}

async function loadAuthorization(): Promise<void> {
  if (userCode.value.length < 6 || loading.value) return
  loading.value = true
  errorMessage.value = ''
  authorization.value = null
  try {
    authorization.value = await agentAPI.getDeviceAuthorization(userCode.value)
  } catch (error) {
    errorMessage.value = authorizationError(error)
  } finally {
    loading.value = false
  }
}

async function approve(): Promise<void> {
  if (!authorization.value || approving.value) return
  approving.value = true
  errorMessage.value = ''
  try {
    await agentAPI.approveDeviceAuthorization(userCode.value)
    approved.value = true
    authorization.value = null
  } catch (error) {
    errorMessage.value = authorizationError(error)
  } finally {
    approving.value = false
  }
}

onMounted(() => {
  const queryCode = typeof route.query.user_code === 'string' ? route.query.user_code : ''
  userCode.value = queryCode.toUpperCase().replace(/[^A-Z0-9_-]/g, '')
  if (userCode.value.length >= 6) void loadAuthorization()
})
</script>
