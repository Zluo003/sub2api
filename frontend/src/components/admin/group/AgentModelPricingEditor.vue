<template>
  <section class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-700">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.groups.agentPricing.title') }}
        </h3>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.groups.agentPricing.subtitle') }}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <button
          type="button"
          class="btn btn-secondary px-3 py-1.5 text-xs"
          :disabled="loading"
          @click="completeDefaults"
        >
          <Icon name="refresh" size="xs" class="mr-1.5" />
          {{ t('admin.groups.agentPricing.completeDefaults') }}
        </button>
        <button
          type="button"
          class="btn btn-secondary px-3 py-1.5 text-xs"
          :title="t('admin.groups.agentPricing.addRule')"
          @click="addRule"
        >
          <Icon name="plus" size="xs" class="mr-1.5" />
          {{ t('common.add') }}
        </button>
      </div>
    </div>

    <div class="inline-flex h-9 items-center border border-gray-200 bg-gray-50 p-0.5 dark:border-dark-600 dark:bg-dark-800" role="tablist">
      <button
        v-for="tab in tabs"
        :key="tab.value"
        type="button"
        role="tab"
        :aria-selected="activeType === tab.value"
        :class="[
          'h-8 px-4 text-xs font-medium transition-colors',
          activeType === tab.value
            ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white'
            : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200',
        ]"
        @click="activeType = tab.value"
      >
        {{ tab.label }}
        <span class="ml-1 text-[10px] text-gray-400">{{ countFor(tab.value) }}</span>
      </button>
    </div>

    <div v-if="loading" class="flex h-28 items-center justify-center text-sm text-gray-400">
      <Icon name="refresh" size="md" class="mr-2 animate-spin" />
      {{ t('common.loading') }}
    </div>

    <div
      v-else-if="visibleRules.length === 0"
      class="flex h-28 items-center justify-center border border-dashed border-gray-300 text-sm text-gray-400 dark:border-dark-600"
    >
      {{ t('admin.groups.agentPricing.empty') }}
    </div>

    <div v-else class="overflow-x-auto border border-gray-200 dark:border-dark-600">
      <table v-if="activeType === 'text'" class="w-full min-w-[1120px] text-left text-xs">
        <thead class="bg-gray-50 text-gray-500 dark:bg-dark-800 dark:text-gray-400">
          <tr>
            <th class="px-3 py-2 font-medium">{{ t('admin.groups.agentPricing.model') }}</th>
            <th class="px-3 py-2 font-medium">{{ t('admin.groups.agentPricing.platform') }}</th>
            <th class="px-3 py-2 font-medium">{{ t('admin.groups.agentPricing.inputPerMillion') }}</th>
            <th class="px-3 py-2 font-medium">{{ t('admin.groups.agentPricing.outputPerMillion') }}</th>
            <th class="px-3 py-2 font-medium">{{ t('admin.groups.agentPricing.cacheWritePerMillion') }}</th>
            <th class="px-3 py-2 font-medium">{{ t('admin.groups.agentPricing.cacheReadPerMillion') }}</th>
            <th class="px-3 py-2 font-medium">{{ t('admin.groups.agentPricing.multiplier') }}</th>
            <th class="w-20 px-3 py-2 font-medium">{{ t('admin.groups.agentPricing.enabled') }}</th>
            <th class="w-12 px-3 py-2"></th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
          <tr v-for="rule in visibleRules" :key="ruleKey(rule)">
            <td class="px-3 py-2"><input :value="rule.model" class="input h-8 min-w-40 text-xs" @input="updateText(rule, 'model', $event)" /></td>
            <td class="px-3 py-2"><select :value="rule.platform" class="input h-8 min-w-28 text-xs" @change="updateText(rule, 'platform', $event)"><option v-for="platform in platforms" :key="platform" :value="platform">{{ platform }}</option></select></td>
            <td class="px-3 py-2"><input :value="rule.input_price_per_million" type="number" min="0" step="0.0001" class="input h-8 w-28 text-xs" @input="updateNumber(rule, 'input_price_per_million', $event)" /></td>
            <td class="px-3 py-2"><input :value="rule.output_price_per_million" type="number" min="0" step="0.0001" class="input h-8 w-28 text-xs" @input="updateNumber(rule, 'output_price_per_million', $event)" /></td>
            <td class="px-3 py-2"><input :value="rule.cache_write_price_per_million" type="number" min="0" step="0.0001" class="input h-8 w-28 text-xs" @input="updateNumber(rule, 'cache_write_price_per_million', $event)" /></td>
            <td class="px-3 py-2"><input :value="rule.cache_read_price_per_million" type="number" min="0" step="0.0001" class="input h-8 w-28 text-xs" @input="updateNumber(rule, 'cache_read_price_per_million', $event)" /></td>
            <td class="px-3 py-2"><input :value="rule.rate_multiplier" type="number" min="0" step="0.01" class="input h-8 w-20 text-xs" @input="updateNumber(rule, 'rate_multiplier', $event)" /></td>
            <td class="px-3 py-2"><input type="checkbox" :checked="rule.enabled" class="h-4 w-4 rounded border-gray-300 text-primary-600" @change="updateEnabled(rule, $event)" /></td>
            <td class="px-3 py-2"><button type="button" class="icon-btn text-gray-400 hover:text-red-600" :title="t('common.delete')" @click="removeRule(rule)"><Icon name="trash" size="sm" /></button></td>
          </tr>
        </tbody>
      </table>

      <table v-else class="w-full min-w-[860px] text-left text-xs">
        <thead class="bg-gray-50 text-gray-500 dark:bg-dark-800 dark:text-gray-400">
          <tr>
            <th class="px-3 py-2 font-medium">{{ t('admin.groups.agentPricing.model') }}</th>
            <th class="px-3 py-2 font-medium">{{ t('admin.groups.agentPricing.platform') }}</th>
            <th class="px-3 py-2 font-medium">{{ t('admin.groups.agentPricing.resolution') }}</th>
            <th class="px-3 py-2 font-medium">{{ activeType === 'image' ? t('admin.groups.agentPricing.perImage') : t('admin.groups.agentPricing.perSecond') }}</th>
            <th class="px-3 py-2 font-medium">{{ t('admin.groups.agentPricing.multiplier') }}</th>
            <th class="px-3 py-2 font-medium">{{ t('admin.groups.agentPricing.effectivePrice') }}</th>
            <th v-if="activeType === 'video'" class="px-3 py-2 font-medium">{{ t('admin.groups.agentPricing.referenceMultiplier') }}</th>
            <th class="w-20 px-3 py-2 font-medium">{{ t('admin.groups.agentPricing.enabled') }}</th>
            <th class="w-12 px-3 py-2"></th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
          <tr v-for="rule in visibleRules" :key="ruleKey(rule)">
            <td class="px-3 py-2"><input :value="rule.model" class="input h-8 min-w-40 text-xs" @input="updateText(rule, 'model', $event)" /></td>
            <td class="px-3 py-2"><select :value="rule.platform" class="input h-8 min-w-28 text-xs" @change="updateText(rule, 'platform', $event)"><option v-for="platform in platforms" :key="platform" :value="platform">{{ platform }}</option></select></td>
            <td class="px-3 py-2"><input :value="rule.resolution" class="input h-8 w-24 text-xs" @input="updateText(rule, 'resolution', $event)" /></td>
            <td class="px-3 py-2"><input :value="rule.unit_price" type="number" min="0" step="0.0001" class="input h-8 w-28 text-xs" @input="updateNumber(rule, 'unit_price', $event)" /></td>
            <td class="px-3 py-2"><input :value="rule.rate_multiplier" type="number" min="0" step="0.01" class="input h-8 w-20 text-xs" @input="updateNumber(rule, 'rate_multiplier', $event)" /></td>
            <td class="px-3 py-2 font-medium text-gray-700 dark:text-gray-200">${{ effectivePrice(rule) }}</td>
            <td v-if="activeType === 'video'" class="px-3 py-2"><input :value="rule.reference_multiplier" type="number" min="0" step="0.01" class="input h-8 w-20 text-xs" @input="updateNumber(rule, 'reference_multiplier', $event)" /></td>
            <td class="px-3 py-2"><input type="checkbox" :checked="rule.enabled" class="h-4 w-4 rounded border-gray-300 text-primary-600" @change="updateEnabled(rule, $event)" /></td>
            <td class="px-3 py-2"><button type="button" class="icon-btn text-gray-400 hover:text-red-600" :title="t('common.delete')" @click="removeRule(rule)"><Icon name="trash" size="sm" /></button></td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@/components/icons'
import type { AgentModelPricingRule, AgentPricingMediaType } from '@/api/admin/agentPricing'

const props = defineProps<{ modelValue: AgentModelPricingRule[]; loading?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: AgentModelPricingRule[]] }>()
const { t } = useI18n()

const activeType = ref<AgentPricingMediaType>('image')
const platforms = ['openai', 'seedance', 'anthropic', 'gemini', 'antigravity', 'grok']
const tabs = computed(() => [
  { value: 'image' as const, label: t('admin.groups.agentPricing.image') },
  { value: 'video' as const, label: t('admin.groups.agentPricing.video') },
  { value: 'text' as const, label: t('admin.groups.agentPricing.text') },
])
const visibleRules = computed(() => props.modelValue.filter((rule) => rule.media_type === activeType.value))

function countFor(type: AgentPricingMediaType) {
  return props.modelValue.filter((rule) => rule.media_type === type).length
}

function emptyRule(type: AgentPricingMediaType): AgentModelPricingRule {
  return {
    model: type === 'image' ? 'gpt-image-2' : type === 'video' ? 'seedance-2.0' : '',
    platform: type === 'video' ? 'seedance' : 'openai',
    media_type: type,
    resolution: type === 'image' ? '2K' : type === 'video' ? '720p' : '',
    unit_price: 0,
    input_price_per_million: 0,
    output_price_per_million: 0,
    cache_write_price_per_million: 0,
    cache_read_price_per_million: 0,
    rate_multiplier: 1,
    reference_multiplier: 1,
    enabled: false,
  }
}

function defaultRules(): AgentModelPricingRule[] {
  return [
    { ...emptyRule('image'), resolution: '2K' },
    { ...emptyRule('image'), resolution: '4K' },
    { ...emptyRule('video'), resolution: '480p' },
    { ...emptyRule('video'), resolution: '720p' },
    { ...emptyRule('video'), resolution: '1080p' },
    { ...emptyRule('video'), resolution: '4K' },
  ]
}

function completeDefaults() {
  const next = [...props.modelValue]
  for (const rule of defaultRules()) {
    const exists = next.some((item) => item.media_type === rule.media_type && item.model === rule.model && item.resolution.toLowerCase() === rule.resolution.toLowerCase())
    if (!exists) next.push(rule)
  }
  emit('update:modelValue', next)
}

function addRule() {
  emit('update:modelValue', [...props.modelValue, emptyRule(activeType.value)])
}

function updateRule(rule: AgentModelPricingRule, patch: Partial<AgentModelPricingRule>) {
  const index = props.modelValue.indexOf(rule)
  if (index < 0) return
  emit('update:modelValue', props.modelValue.map((item, itemIndex) => itemIndex === index ? { ...item, ...patch } : item))
}

function updateText(rule: AgentModelPricingRule, field: 'model' | 'platform' | 'resolution', event: Event) {
  updateRule(rule, { [field]: (event.target as HTMLInputElement | HTMLSelectElement).value })
}

function updateNumber(rule: AgentModelPricingRule, field: keyof AgentModelPricingRule, event: Event) {
  const value = Number((event.target as HTMLInputElement).value)
  updateRule(rule, { [field]: Number.isFinite(value) && value >= 0 ? value : 0 })
}

function updateEnabled(rule: AgentModelPricingRule, event: Event) {
  updateRule(rule, { enabled: (event.target as HTMLInputElement).checked })
}

function removeRule(rule: AgentModelPricingRule) {
  emit('update:modelValue', props.modelValue.filter((item) => item !== rule))
}

function effectivePrice(rule: AgentModelPricingRule) {
  return (rule.unit_price * rule.rate_multiplier).toFixed(4)
}

function ruleKey(rule: AgentModelPricingRule) {
  return rule.id || `${rule.media_type}:${rule.model}:${rule.resolution}:${props.modelValue.indexOf(rule)}`
}
</script>
