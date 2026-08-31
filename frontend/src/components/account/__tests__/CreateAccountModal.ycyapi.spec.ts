import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const createSource = readFileSync(
  resolve(process.cwd(), 'src/components/account/CreateAccountModal.vue'),
  'utf8'
)
const editSource = readFileSync(
  resolve(process.cwd(), 'src/components/account/EditAccountModal.vue'),
  'utf8'
)

describe('Seedance YCYAPI account defaults', () => {
  it('maps all downstream model names only in the YCYAPI upstream account config', () => {
    for (const source of [createSource, editSource]) {
      expect(source).toContain("{ from: 'seedance-2.0', to: 'firefly-video-v2' }")
      expect(source).toContain("{ from: 'seedance-2.0-fast', to: 'firefly-video-v2-fast' }")
      expect(source).toContain("{ from: 'seedance-2.5', to: 'leonardo-seedance-2.5' }")
    }
  })

  it('uses the YCYAPI endpoint and async task timeouts', () => {
    expect(createSource).toContain("baseUrl: 'https://ycyapi.cn'")
    expect(createSource).toContain("apiPath: '/v1/videos'")
    expect(createSource).toContain('pollIntervalMs: 5000')
    expect(createSource).toContain('pollTimeoutMs: 3600000')
    expect(createSource).toContain('requestTimeoutMs: 300000')
  })
})
