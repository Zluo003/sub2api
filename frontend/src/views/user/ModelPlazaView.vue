<template>
  <AppLayout>
    <div class="mx-auto max-w-[1480px] space-y-5">
      <header class="flex flex-col justify-between gap-4 border-b border-gray-200 pb-5 dark:border-dark-700 md:flex-row md:items-end">
        <div>
          <div class="flex items-center gap-2">
            <Icon name="grid" size="lg" class="text-primary-500" />
            <h1 class="text-2xl font-semibold text-gray-950 dark:text-white">{{ t('modelPlaza.title') }}</h1>
          </div>
          <p class="mt-2 max-w-2xl text-sm leading-6 text-gray-500 dark:text-dark-400">
            {{ t('modelPlaza.description') }}
          </p>
        </div>
        <div class="flex items-center gap-4 text-sm text-gray-500 dark:text-dark-400">
          <span><strong class="font-semibold text-gray-900 dark:text-white">{{ filteredModels.length }}</strong> {{ t('modelPlaza.models') }}</span>
          <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadModels">
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
            {{ t('common.refresh') }}
          </button>
        </div>
      </header>

      <div class="flex flex-col gap-5 lg:flex-row">
        <aside class="shrink-0 lg:sticky lg:top-24 lg:w-60 lg:self-start">
          <div class="space-y-4 rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
            <div>
              <label for="model-search" class="mb-2 block text-xs font-medium text-gray-600 dark:text-dark-300">
                {{ t('modelPlaza.search') }}
              </label>
              <div class="relative">
                <Icon name="search" size="sm" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                <input
                  id="model-search"
                  v-model="query"
                  type="search"
                  class="input h-10 pl-9 text-sm"
                  :placeholder="t('modelPlaza.searchPlaceholder')"
                />
              </div>
            </div>

            <div>
              <div class="mb-2 flex items-center justify-between text-xs font-medium text-gray-600 dark:text-dark-300">
                <span>{{ t('modelPlaza.vendor') }}</span>
                <span class="font-mono text-gray-400">{{ vendors.length }}</span>
              </div>
              <select v-model="selectedVendor" class="input h-10 text-sm lg:hidden">
                <option value="all">{{ t('modelPlaza.allVendors') }} ({{ models.length }})</option>
                <option v-for="vendor in vendors" :key="vendor.name" :value="vendor.name">
                  {{ vendor.name }} ({{ vendor.count }})
                </option>
              </select>
              <div class="hidden space-y-1 lg:block">
                <button
                  type="button"
                  class="vendor-filter"
                  :class="selectedVendor === 'all' ? 'vendor-filter-active' : ''"
                  @click="selectedVendor = 'all'"
                >
                  <span>{{ t('modelPlaza.allVendors') }}</span>
                  <span class="font-mono text-xs">{{ models.length }}</span>
                </button>
                <button
                  v-for="vendor in vendors"
                  :key="vendor.name"
                  type="button"
                  class="vendor-filter"
                  :class="selectedVendor === vendor.name ? 'vendor-filter-active' : ''"
                  @click="selectedVendor = vendor.name"
                >
                  <span class="truncate">{{ vendor.name }}</span>
                  <span class="font-mono text-xs">{{ vendor.count }}</span>
                </button>
              </div>
            </div>

            <button
              v-if="query || selectedVendor !== 'all'"
              type="button"
              class="btn btn-secondary w-full justify-center"
              @click="clearFilters"
            >
              {{ t('modelPlaza.clearFilters') }}
            </button>
          </div>
        </aside>

        <main class="min-w-0 flex-1">
          <div v-if="loading" class="flex min-h-[420px] items-center justify-center">
            <Icon name="refresh" size="xl" class="animate-spin text-primary-500" />
          </div>

          <div v-else-if="filteredModels.length === 0" class="flex min-h-[420px] flex-col items-center justify-center text-center">
            <Icon name="search" size="xl" class="mb-3 text-gray-300 dark:text-dark-500" />
            <p class="font-medium text-gray-800 dark:text-gray-200">{{ t('modelPlaza.empty') }}</p>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('modelPlaza.emptyHint') }}</p>
          </div>

          <div v-else class="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">
            <article
              v-for="model in filteredModels"
              :key="`${model.platform}-${model.name}`"
              class="group flex min-h-[300px] flex-col rounded-lg border border-gray-200 bg-white p-5 transition duration-200 hover:-translate-y-0.5 hover:border-primary-300 hover:shadow-lg hover:shadow-gray-200/50 dark:border-dark-700 dark:bg-dark-800 dark:hover:border-primary-700 dark:hover:shadow-black/20"
            >
              <div class="flex items-start justify-between gap-3">
                <div class="flex min-w-0 items-start gap-3">
                  <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-gray-200 bg-gray-50 text-sm font-bold text-gray-700 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200">
                    {{ identity(model).vendor.slice(0, 1) }}
                  </div>
                  <div class="min-w-0">
                    <p class="text-xs font-medium text-primary-600 dark:text-primary-400">{{ identity(model).vendor }}</p>
                    <h2 class="mt-0.5 break-words font-mono text-sm font-semibold text-gray-950 dark:text-white">{{ model.name }}</h2>
                  </div>
                </div>
                <span class="inline-flex shrink-0 items-center gap-1 rounded-md border px-2 py-1 text-[11px] font-medium" :class="platformBadgeClass(model.platform)">
                  <PlatformIcon :platform="model.platform as GroupPlatform" size="xs" />
                  {{ platformLabel(model.platform) }}
                </span>
              </div>

              <p class="mt-4 min-h-[48px] text-sm leading-6 text-gray-600 dark:text-gray-300">
                {{ locale === 'zh' ? identity(model).descriptionZh : identity(model).descriptionEn }}
              </p>

              <div class="mt-4 border-t border-gray-100 pt-4 dark:border-dark-700">
                <div class="mb-2 flex items-center justify-between">
                  <h3 class="text-xs font-semibold uppercase text-gray-500 dark:text-dark-400">{{ t('modelPlaza.standardPricing') }}</h3>
                  <span class="text-[11px] text-gray-400">{{ billingModeLabel(model.pricing?.billing_mode) }}</span>
                </div>
                <div v-if="pricingRows(model).length" class="grid grid-cols-2 gap-x-4 gap-y-2">
                  <div v-for="row in pricingRows(model)" :key="row.label" class="min-w-0">
                    <p class="truncate text-[11px] text-gray-500 dark:text-dark-400">{{ row.label }}</p>
                    <p class="mt-0.5 truncate font-mono text-xs font-semibold text-gray-900 dark:text-white">{{ row.value }}</p>
                  </div>
                </div>
                <p v-else class="text-sm text-gray-400">{{ t('modelPlaza.noPricing') }}</p>
              </div>

              <footer class="mt-auto flex items-center justify-between gap-3 border-t border-gray-100 pt-4 text-xs text-gray-500 dark:border-dark-700 dark:text-dark-400">
                <span>{{ t('modelPlaza.availableGroups', { count: model.group_count }) }}</span>
                <button
                  type="button"
                  class="rounded-md p-2 text-gray-500 transition hover:bg-gray-100 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white"
                  :title="copiedModel === model.name ? t('modelPlaza.copied') : t('modelPlaza.copyModel')"
                  @click="copyModel(model.name)"
                >
                  <Icon :name="copiedModel === model.name ? 'check' : 'copy'" size="sm" />
                </button>
              </footer>
            </article>
          </div>
        </main>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import { userChannelsAPI, type UserModelPlazaItem, type UserSupportedModelPricing } from '@/api/channels'
import type { BillingMode } from '@/constants/channel'
import type { GroupPlatform } from '@/types'
import { formatScaled } from '@/utils/pricing'
import { getModelIdentity } from '@/utils/modelCatalog'
import { platformBadgeClass, platformLabel } from '@/utils/platformColors'

const { t, locale } = useI18n()
const models = ref<UserModelPlazaItem[]>([])
const loading = ref(true)
const query = ref('')
const selectedVendor = ref('all')
const copiedModel = ref('')

const identity = (model: UserModelPlazaItem) => getModelIdentity(model.name, model.platform)

const vendors = computed(() => {
  const counts = new Map<string, number>()
  for (const model of models.value) {
    const vendor = identity(model).vendor
    counts.set(vendor, (counts.get(vendor) || 0) + 1)
  }
  return [...counts.entries()]
    .map(([name, count]) => ({ name, count }))
    .sort((a, b) => a.name.localeCompare(b.name))
})

const filteredModels = computed(() => {
  const needle = query.value.trim().toLowerCase()
  return models.value.filter((model) => {
    const modelIdentity = identity(model)
    if (selectedVendor.value !== 'all' && modelIdentity.vendor !== selectedVendor.value) return false
    if (!needle) return true
    return [model.name, model.platform, modelIdentity.vendor, modelIdentity.descriptionZh, modelIdentity.descriptionEn]
      .some((value) => value.toLowerCase().includes(needle))
  })
})

async function loadModels() {
  loading.value = true
  try {
    models.value = await userChannelsAPI.getModelPlaza()
  } catch (error) {
    console.error('Failed to load model plaza:', error)
    models.value = []
  } finally {
    loading.value = false
  }
}

function clearFilters() {
  query.value = ''
  selectedVendor.value = 'all'
}

async function copyModel(name: string) {
  await navigator.clipboard.writeText(name)
  copiedModel.value = name
  window.setTimeout(() => {
    if (copiedModel.value === name) copiedModel.value = ''
  }, 1500)
}

function billingModeLabel(mode?: BillingMode) {
  if (!mode) return t('modelPlaza.noPricing')
  return t(`modelPlaza.billingModes.${mode}`)
}

function pricingRows(model: UserModelPlazaItem) {
  const pricing = model.pricing
  if (!pricing) return []

  if (pricing.billing_mode === 'token') {
    return [
      priceRow(t('modelPlaza.pricing.input'), pricing.input_price, 1_000_000, t('modelPlaza.perMillion')),
      priceRow(t('modelPlaza.pricing.output'), pricing.output_price, 1_000_000, t('modelPlaza.perMillion')),
      priceRow(t('modelPlaza.pricing.cacheRead'), pricing.cache_read_price, 1_000_000, t('modelPlaza.perMillion')),
      priceRow(t('modelPlaza.pricing.cacheWrite'), pricing.cache_write_price, 1_000_000, t('modelPlaza.perMillion'))
    ].filter(isPriceRow)
  }

  const unit = pricing.billing_mode === 'image'
    ? t('modelPlaza.perImage')
    : pricing.billing_mode === 'video_duration'
      ? t('modelPlaza.perSecond')
      : t('modelPlaza.perRequest')
  const rows = [priceRow(t('modelPlaza.pricing.default'), pricing.per_request_price, 1, unit)]
  for (const interval of pricing.intervals || []) {
    rows.push(priceRow(interval.tier_label || intervalLabel(interval), interval.per_request_price, 1, unit))
  }
  return rows.filter(isPriceRow).slice(0, 4)
}

type PriceRow = { label: string; value: string } | null
function priceRow(label: string, price: number | null, scale: number, unit: string): PriceRow {
  if (price == null) return null
  return { label, value: `${formatScaled(price, scale)} ${unit}` }
}

function isPriceRow(row: PriceRow): row is NonNullable<PriceRow> {
  return row !== null
}

function intervalLabel(interval: UserSupportedModelPricing['intervals'][number]) {
  return interval.max_tokens == null
    ? `>${interval.min_tokens}`
    : `${interval.min_tokens}-${interval.max_tokens}`
}

onMounted(loadModels)
</script>

<style scoped>
.vendor-filter {
  @apply flex min-w-[140px] items-center justify-between gap-3 rounded-md px-3 py-2 text-left text-sm text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-950 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white lg:w-full;
}

.vendor-filter-active {
  @apply bg-primary-50 font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300;
}
</style>
