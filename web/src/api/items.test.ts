import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fetchItems } from './items'

function mockJson(body: unknown) {
  globalThis.fetch = vi.fn().mockResolvedValue({
    ok: true,
    json: async () => body,
  }) as unknown as typeof fetch
}

describe('fetchItems', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('parses the ItemList envelope', async () => {
    mockJson({
      count: 1, hasNext: true, nextCursor: 'CUR',
      items: [{ id: 'a', title: '标题', url: 'https://x/a', permalink: '/items/a', source: 'Src', selected: true }],
    })
    const out = await fetchItems({})
    expect(out.count).toBe(1)
    expect(out.hasNext).toBe(true)
    expect(out.nextCursor).toBe('CUR')
    expect(out.items[0].title).toBe('标题')
  })

  it('builds the query string from params (only set ones)', async () => {
    mockJson({ count: 0, hasNext: false, nextCursor: null, items: [] })
    await fetchItems({ q: 'OpenAI', category: 'ai-models', take: 20, cursor: 'C1', mode: 'all' })
    const url = (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0] as string
    expect(url).toContain('/api/public/items?')
    expect(url).toContain('q=OpenAI')
    expect(url).toContain('category=ai-models')
    expect(url).toContain('take=20')
    expect(url).toContain('cursor=C1')
    expect(url).toContain('mode=all')
  })

  it('omits unset params', async () => {
    mockJson({ count: 0, hasNext: false, nextCursor: null, items: [] })
    await fetchItems({ take: 10 })
    const url = (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0] as string
    expect(url).toContain('take=10')
    expect(url).not.toContain('q=')
    expect(url).not.toContain('category=')
    expect(url).not.toContain('cursor=')
  })

  it('throws on non-ok', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 400 }) as unknown as typeof fetch
    await expect(fetchItems({})).rejects.toThrow(/400/)
  })
})
