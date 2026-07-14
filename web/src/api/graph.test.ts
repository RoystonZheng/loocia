import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fetchCloud, fetchTerm } from './graph'

describe('graph api', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('fetchCloud hits the cloud endpoint with the window', async () => {
    const mock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ terms: [] }) })
    globalThis.fetch = mock as unknown as typeof fetch
    await fetchCloud('30d')
    expect(mock).toHaveBeenCalledWith('/api/public/graph/cloud?window=30d')
  })

  it('fetchTerm URL-encodes the term', async () => {
    const mock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ term: '推理', kind: 'topic', count: 1, neighbors: [], items: [] }) })
    globalThis.fetch = mock as unknown as typeof fetch
    await fetchTerm('推理模型', '7d')
    expect(mock).toHaveBeenCalledWith(`/api/public/graph/term/${encodeURIComponent('推理模型')}?window=7d`)
  })

  it('throws on non-ok responses', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 }) as unknown as typeof fetch
    await expect(fetchCloud('7d')).rejects.toThrow('500')
    await expect(fetchTerm('x', '7d')).rejects.toThrow('500')
  })
})
