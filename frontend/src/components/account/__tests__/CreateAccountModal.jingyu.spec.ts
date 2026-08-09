import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  resolve(process.cwd(), 'src/components/account/CreateAccountModal.vue'),
  'utf8'
)

describe('CreateAccountModal Jingyu model defaults', () => {
  it('maps Seedance 2.0 and 2.5 to the Jingyu upstream models', () => {
    expect(source).toContain("{ from: 'seedance-2.0', to: 'yu-video-2-pro' }")
    expect(source).toContain("{ from: 'seedance-2.5', to: 'yu-video-2.5-pro' }")
    expect(source).not.toContain("to: 'jing-video-2-pro'")
  })

  it('does not submit the unsupported upstream billing probe for Seedance accounts', () => {
    expect(source).toContain("const upstreamBillingProbeSupported = computed(() => form.platform !== 'seedance')")
    expect(source).toContain('upstream_billing_probe_enabled: upstreamBillingProbeSupported.value ? upstreamBillingAutoProbeEnabled.value : undefined')
  })

  it('uses the NewWaule Jingyu timeout defaults', () => {
    expect(source).toContain('pollIntervalMs: 5000')
    expect(source).toContain('pollTimeoutMs: 1800000')
    expect(source).toContain('requestTimeoutMs: 1800000')
    expect(source).toContain('connectTimeoutMs: 60000')
  })
})
