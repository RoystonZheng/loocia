import { describe, it, expect, vi } from 'vitest'
import { fetchVersion } from './version'

describe('fetchVersion', () => {
  it('returns parsed PublicVersion', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ apiVersion: '1.1.0', skillVersion: '0.1.0', updatedAt: '2026-07-03', changelogUrl: '/changelog', recentChanges: [] }),
    }) as unknown as typeof fetch
    const v = await fetchVersion()
    expect(v.apiVersion).toBe('1.1.0')
  })
})
