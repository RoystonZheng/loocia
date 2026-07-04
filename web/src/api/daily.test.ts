import { describe, it, expect, vi } from 'vitest'
import { fetchLatestDaily } from './daily'

describe('fetchLatestDaily', () => {
  it('returns the report', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        date: '2026-05-07', generatedAt: '2026-05-08T01:00:00Z',
        windowStart: '2026-05-07T00:00:00Z', windowEnd: '2026-05-08T00:00:00Z',
        lead: { title: '导语', leadParagraph: '段落' }, sections: [], flashes: [],
      }),
    }) as unknown as typeof fetch
    const rep = await fetchLatestDaily()
    expect(rep?.date).toBe('2026-05-07')
    expect(rep?.lead?.title).toBe('导语')
  })

  it('returns null on 404', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 404 }) as unknown as typeof fetch
    expect(await fetchLatestDaily()).toBeNull()
  })

  it('throws on other errors', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 }) as unknown as typeof fetch
    await expect(fetchLatestDaily()).rejects.toThrow(/500/)
  })
})
