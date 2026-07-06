import { describe, it, expect, vi } from 'vitest'
import { fetchHotTopics } from './hot'

describe('fetchHotTopics', () => {
  it('returns the list', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ count: 1, items: [{ id: 'h1', title: '热点', url: 'https://x/h1', permalink: '/items/h1', source: 'S', sourceCount: 3, sourceNames: ['S'], latestAt: '2026-05-07T10:00:00Z' }] }),
    }) as unknown as typeof fetch
    const out = await fetchHotTopics()
    expect(out.count).toBe(1)
    expect(out.items[0].sourceCount).toBe(3)
  })

  it('throws on non-ok', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 }) as unknown as typeof fetch
    await expect(fetchHotTopics()).rejects.toThrow(/500/)
  })
})
