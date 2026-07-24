<template>
  <section data-testid="agent-model-catalog" class="border-t border-gray-200 pt-5 dark:border-dark-600">
    <div class="flex items-start justify-between gap-4">
      <div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.groups.agentCatalog.title') }}
        </h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.groups.agentCatalog.description') }}
        </p>
      </div>
      <button
        type="button"
        data-testid="sync-agent-models"
        class="btn btn-secondary btn-sm flex-shrink-0"
        :disabled="loading || syncing"
        @click="syncModels"
      >
        <Icon name="refresh" size="sm" :class="syncing ? 'animate-spin' : ''" />
        {{ t('admin.groups.agentCatalog.sync') }}
      </button>
    </div>

    <div class="mt-5 border-y border-gray-200 py-4 dark:border-dark-600">
      <div class="mb-3 flex items-center justify-between gap-3">
        <div>
          <p class="text-sm font-medium text-gray-800 dark:text-gray-200">
            {{ t('admin.groups.agentCatalog.languageRates') }}
          </p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.groups.agentCatalog.languageRatesHint') }}
          </p>
        </div>
        <button
          type="button"
          data-testid="save-agent-platform-rates"
          class="btn btn-secondary btn-sm"
          :disabled="savingRates"
          @click="saveRates"
        >
          {{ savingRates ? t('common.saving') : t('admin.groups.agentCatalog.saveRates') }}
        </button>
      </div>
      <div class="grid grid-cols-3 gap-3">
        <label v-for="platform in languagePlatforms" :key="platform" class="block">
          <span class="input-label">{{ platformLabels[platform] }}</span>
          <input
            v-model="rateDrafts[platform]"
            type="number"
            min="0"
            step="0.001"
            class="input"
            :data-testid="`agent-rate-${platform}`"
            :placeholder="t('admin.groups.agentCatalog.notConfigured')"
          />
        </label>
      </div>
    </div>

    <div class="mt-4 flex items-center justify-between gap-3">
      <div class="flex items-center gap-2 text-sm font-medium text-gray-800 dark:text-gray-200">
        <span>{{ t('admin.groups.agentCatalog.models') }}</span>
        <span class="text-xs font-normal text-gray-500 dark:text-gray-400">{{ modelDrafts.length }}</span>
      </div>
      <select v-model="mediaFilter" class="input w-36">
        <option value="all">{{ t('admin.groups.agentCatalog.allTypes') }}</option>
        <option value="text">{{ t('admin.groups.agentCatalog.types.text') }}</option>
        <option value="image">{{ t('admin.groups.agentCatalog.types.image') }}</option>
        <option value="video">{{ t('admin.groups.agentCatalog.types.video') }}</option>
      </select>
    </div>

    <div v-if="loading" class="py-10 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('common.loading') }}
    </div>
    <div
      v-else-if="modelDrafts.length === 0"
      class="mt-3 border-y border-dashed border-gray-300 py-10 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
    >
      {{ t('admin.groups.agentCatalog.empty') }}
    </div>
    <div v-else class="mt-3 max-h-[34rem] overflow-auto border-y border-gray-200 dark:border-dark-600">
      <div
        v-for="model in filteredModels"
        :key="model.id"
        :data-testid="`agent-model-${model.id}`"
        class="border-b border-gray-100 px-1 py-4 last:border-b-0 dark:border-dark-700"
      >
        <div class="grid grid-cols-[minmax(12rem,1fr)_8rem_8rem_6rem_auto] items-center gap-3">
          <div class="min-w-0">
            <div class="break-all font-mono text-sm text-gray-900 dark:text-gray-100">
              {{ model.model_code }}
            </div>
            <div class="mt-1 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
              <span>{{ platformLabels[model.platform] }}</span>
              <span :class="model.available ? 'text-emerald-600 dark:text-emerald-400' : 'text-amber-600 dark:text-amber-400'">
                {{ model.available ? t('admin.groups.agentCatalog.available') : t('admin.groups.agentCatalog.unavailable') }}
              </span>
            </div>
          </div>
          <select
            v-model="model.media_type"
            class="input"
            :data-testid="`agent-model-type-${model.id}`"
            @change="resetPricesForType(model)"
          >
            <option value="text">{{ t('admin.groups.agentCatalog.types.text') }}</option>
            <option value="image">{{ t('admin.groups.agentCatalog.types.image') }}</option>
            <option value="video">{{ t('admin.groups.agentCatalog.types.video') }}</option>
          </select>
          <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input
              v-model="model.enabled"
              type="checkbox"
              :data-testid="`agent-model-enabled-${model.id}`"
              class="h-4 w-4 rounded border-gray-300 text-cyan-600 focus:ring-cyan-500"
            />
            {{ t('admin.groups.agentCatalog.enabled') }}
          </label>
          <button
            type="button"
            :data-testid="`save-agent-model-${model.id}`"
            class="btn btn-secondary btn-sm"
            :disabled="savingModelId === model.id"
            @click="saveModel(model)"
          >
            {{ savingModelId === model.id ? t('common.saving') : t('common.save') }}
          </button>
          <button
            type="button"
            :data-testid="`exclude-agent-model-${model.id}`"
            class="rounded p-2 text-gray-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-950/30 dark:hover:text-red-400"
            :title="t('admin.groups.agentCatalog.exclude')"
            :disabled="deletingModelId === model.id"
            @click="excludeModel(model)"
          >
            <Icon name="trash" size="sm" />
          </button>
        </div>

        <div v-if="model.media_type === 'image'" class="mt-3 grid grid-cols-3 gap-3 pl-0">
          <label v-for="price in model.prices" :key="price.resolution" class="block">
            <span class="input-label">{{ price.resolution }} / {{ t('admin.groups.agentCatalog.perImage') }}</span>
            <input
              v-model="price.unit_price"
              type="number"
              min="0"
              step="0.0001"
              class="input"
              :data-testid="`agent-model-price-${model.id}-${price.resolution}`"
              :placeholder="t('admin.groups.agentCatalog.notConfigured')"
            />
          </label>
        </div>

        <div v-else-if="model.media_type === 'video'" class="mt-3 pl-0">
          <div
            v-for="(price, index) in model.prices"
            :key="`${model.id}-${index}`"
            class="mb-2 grid grid-cols-[10rem_1fr_auto] items-end gap-3 last:mb-0"
          >
            <label class="block">
              <span class="input-label">{{ t('admin.groups.agentCatalog.resolution') }}</span>
              <input
                v-model.trim="price.resolution"
                type="text"
                class="input"
                placeholder="1080p"
                :data-testid="`agent-video-resolution-${model.id}-${index}`"
              />
            </label>
            <label class="block">
              <span class="input-label">{{ t('admin.groups.agentCatalog.perSecond') }}</span>
              <input
                v-model="price.unit_price"
                type="number"
                min="0"
                step="0.0001"
                class="input"
                :data-testid="`agent-video-price-${model.id}-${index}`"
                :placeholder="t('admin.groups.agentCatalog.notConfigured')"
              />
            </label>
            <button
              type="button"
              class="mb-1 rounded p-2 text-gray-400 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-600 dark:hover:text-gray-200"
              :title="t('common.delete')"
              @click="model.prices.splice(index, 1)"
            >
              <Icon name="trash" size="sm" />
            </button>
          </div>
          <button
            type="button"
            class="mt-2 inline-flex items-center gap-1 text-xs font-medium text-cyan-700 hover:text-cyan-800 dark:text-cyan-400"
            :data-testid="`add-agent-video-price-${model.id}`"
            @click="addVideoPrice(model)"
          >
            <Icon name="plus" size="xs" />
            {{ t('admin.groups.agentCatalog.addResolution') }}
          </button>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import Icon from '@/components/icons/Icon.vue'
import type {
  AgentGroupModel,
  AgentLanguagePlatform,
  AgentMediaType,
  AgentModelConfig,
  AgentModelPlatform
} from '@/api/admin/groups'

const props = defineProps<{ groupId: number }>()
const { t } = useI18n()
const appStore = useAppStore()

type PriceDraft = { resolution: string; unit_price: number | string }
type ModelDraft = Omit<AgentGroupModel, 'prices'> & { prices: PriceDraft[] }

const languagePlatforms: AgentLanguagePlatform[] = ['openai', 'anthropic', 'gemini']
const platformLabels: Record<AgentModelPlatform, string> = {
  openai: 'OpenAI',
  anthropic: 'Anthropic',
  gemini: 'Gemini',
  seedance: 'Seedance'
}
const loading = ref(false)
const syncing = ref(false)
const savingRates = ref(false)
const savingModelId = ref<number | null>(null)
const deletingModelId = ref<number | null>(null)
const mediaFilter = ref<'all' | AgentMediaType>('all')
const modelDrafts = ref<ModelDraft[]>([])
const rateDrafts = reactive<Record<AgentLanguagePlatform, number | string>>({
  openai: '',
  anthropic: '',
  gemini: ''
})

const filteredModels = computed(() =>
  mediaFilter.value === 'all'
    ? modelDrafts.value
    : modelDrafts.value.filter(model => model.media_type === mediaFilter.value)
)

function applyConfig(config: AgentModelConfig) {
  for (const platform of languagePlatforms) {
    const rate = config.platform_rates.find(item => item.platform === platform)
    rateDrafts[platform] = rate?.rate_multiplier ?? ''
  }
  modelDrafts.value = config.models.map(model => ({
    ...model,
    prices: pricesForDraft(model)
  }))
}

function pricesForDraft(model: AgentGroupModel): PriceDraft[] {
  if (model.media_type === 'image') {
    return ['1K', '2K', '4K'].map(resolution => ({
      resolution,
      unit_price: model.prices.find(price => price.resolution === resolution)?.unit_price ?? ''
    }))
  }
  if (model.media_type === 'video') {
    return model.prices.map(price => ({ resolution: price.resolution, unit_price: price.unit_price }))
  }
  return []
}

async function load() {
  if (!props.groupId) return
  loading.value = true
  try {
    applyConfig(await adminAPI.groups.getAgentModels(props.groupId))
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.groups.agentCatalog.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function syncModels() {
  syncing.value = true
  try {
    applyConfig(await adminAPI.groups.syncAgentModels(props.groupId))
    appStore.showSuccess(t('admin.groups.agentCatalog.syncSuccess'))
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.groups.agentCatalog.syncFailed'))
  } finally {
    syncing.value = false
  }
}

async function saveRates() {
  const configured = languagePlatforms.filter(platform => rateDrafts[platform] !== '')
  if (configured.some(platform => Number(rateDrafts[platform]) < 0 || !Number.isFinite(Number(rateDrafts[platform])))) {
    appStore.showError(t('admin.groups.agentCatalog.invalidRate'))
    return
  }
  const updates = configured.map(platform => ({
    platform,
    multiplier: Number(rateDrafts[platform])
  }))
  savingRates.value = true
  try {
    let config: AgentModelConfig | null = null
    for (const update of updates) {
      config = await adminAPI.groups.setAgentPlatformRate(props.groupId, update.platform, update.multiplier)
    }
    if (config) applyConfig(config)
    appStore.showSuccess(t('admin.groups.agentCatalog.ratesSaved'))
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.groups.agentCatalog.saveFailed'))
  } finally {
    savingRates.value = false
  }
}

function resetPricesForType(model: ModelDraft) {
  if (model.media_type === 'image') {
    model.prices = ['1K', '2K', '4K'].map(resolution => ({ resolution, unit_price: '' }))
  } else {
    model.prices = []
  }
}

function addVideoPrice(model: ModelDraft) {
  model.prices.push({ resolution: '', unit_price: '' })
}

async function saveModel(model: ModelDraft) {
  const prices = model.media_type === 'text'
    ? []
    : model.prices
        .filter(price => price.resolution.trim() !== '' && price.unit_price !== '')
        .map(price => ({ resolution: price.resolution.trim(), unit_price: Number(price.unit_price) }))
  if (prices.some(price => !Number.isFinite(price.unit_price) || price.unit_price < 0)) {
    appStore.showError(t('admin.groups.agentCatalog.invalidPrice'))
    return
  }
  savingModelId.value = model.id
  try {
    applyConfig(await adminAPI.groups.updateAgentModel(props.groupId, model.id, {
      media_type: model.media_type,
      enabled: model.enabled,
      prices
    }))
    appStore.showSuccess(t('admin.groups.agentCatalog.modelSaved'))
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.groups.agentCatalog.saveFailed'))
  } finally {
    savingModelId.value = null
  }
}

async function excludeModel(model: ModelDraft) {
  if (!window.confirm(t('admin.groups.agentCatalog.excludeConfirm', { model: model.model_code }))) return
  deletingModelId.value = model.id
  try {
    applyConfig(await adminAPI.groups.deleteAgentModel(props.groupId, model.id))
    appStore.showSuccess(t('admin.groups.agentCatalog.excluded'))
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.groups.agentCatalog.deleteFailed'))
  } finally {
    deletingModelId.value = null
  }
}

watch(() => props.groupId, load, { immediate: true })
</script>
