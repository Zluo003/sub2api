import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PricingEntryCard from '../PricingEntryCard.vue'
import type { PricingFormEntry } from '../types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

function entry(): PricingFormEntry {
  return {
    models: [],
    billing_mode: 'token',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: []
  }
}

const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue'],
  template: `
    <select data-testid="billing-mode" :value="modelValue" @change="$emit('update:modelValue', $event.target.value)">
      <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
    </select>
  `
}

describe('PricingEntryCard video pricing', () => {
  it('Seedance 提供按 Token、按次、按分辨率/秒三种配置，并可新增 480p 档位', async () => {
    const wrapper = mount(PricingEntryCard, {
      props: { entry: entry(), platform: 'seedance' },
      global: {
        stubs: {
          Select: SelectStub,
          Icon: true,
          ModelTagInput: true,
          IntervalRow: true
        }
      }
    })

    const select = wrapper.get('[data-testid="billing-mode"]')
    expect(select.findAll('option').map(option => option.attributes('value'))).toEqual([
      'token',
      'per_request',
      'video_duration'
    ])

    await select.setValue('video_duration')
    const modeUpdate = wrapper.emitted('update')?.at(-1)?.[0] as PricingFormEntry
    await wrapper.setProps({ entry: modeUpdate })

    const addButton = wrapper.findAll('button').find(button =>
      button.text().includes('admin.channels.form.addResolution'))
    expect(addButton).toBeDefined()
    await addButton!.trigger('click')

    const resolutionUpdate = wrapper.emitted('update')?.at(-1)?.[0] as PricingFormEntry
    expect(resolutionUpdate.billing_mode).toBe('video_duration')
    expect(resolutionUpdate.intervals).toHaveLength(1)
    expect(resolutionUpdate.intervals[0].tier_label).toBe('480p')
    expect(resolutionUpdate.intervals[0].per_request_price).toBeNull()
  })

  it('非视频平台保持 Token、按次、图片三种配置', () => {
    const wrapper = mount(PricingEntryCard, {
      props: { entry: entry(), platform: 'openai' },
      global: {
        stubs: {
          Select: SelectStub,
          Icon: true,
          ModelTagInput: true,
          IntervalRow: true
        }
      }
    })

    expect(wrapper.get('[data-testid="billing-mode"]').findAll('option').map(option => option.attributes('value'))).toEqual([
      'token',
      'per_request',
      'image'
    ])
  })
})
