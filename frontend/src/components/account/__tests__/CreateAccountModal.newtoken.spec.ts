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

describe('Seedance newtoken account defaults', () => {
  it('offers newtoken as an upstream video provider in both modals', () => {
    for (const source of [createSource, editSource]) {
      expect(source).toContain("type VideoProvider = 'aigod' | 'ycyapi' | 'jingyu' | 'newtoken' | 'mikuapi'")
      expect(source).toContain('admin.accounts.video.providers.newtoken')
    }
    expect(createSource).toContain("videoProvider = 'newtoken'")
    expect(editSource).toContain('<option value="newtoken">')
    // Without this branch the edit modal would render a stored newtoken account as aigod.
    expect(editSource).toContain("extra?.video_provider === 'newtoken'")
  })

  it('uses the newtoken endpoint and async task timeouts', () => {
    for (const source of [createSource, editSource]) {
      expect(source).toContain("baseUrl: 'https://newtoken.club'")
      expect(source).toContain("apiPath: '/v1/videos'")
      expect(source).toContain('pollIntervalMs: 5000')
      expect(source).toContain('pollTimeoutMs: 3600000')
      expect(source).toContain('requestTimeoutMs: 300000')
      expect(source).toContain('connectTimeoutMs: 15000')
    }
  })

  it('never prefills a static model mapping for newtoken', () => {
    // newtoken bakes the resolution into the upstream model id, so routing must stay
    // in the backend adapter. A static mapping here would take precedence over it and
    // silently send every 1080p request to the 720p upstream model.
    for (const source of [createSource, editSource]) {
      expect(source).not.toMatch(/to:\s*'sd\d/)
    }
  })
})
