import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { Feed } from './Feed'
import type { ItemList } from '../api/items'

function page(items: string[], nextCursor: string | null): ItemList {
  return {
    count: items.length,
    hasNext: nextCursor !== null,
    nextCursor,
    items: items.map((id) => ({
      id, title: 't-' + id, url: 'https://x/' + id, permalink: '/items/' + id,
      source: 'S', selected: true,
    })),
  }
}

describe('Feed', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('loads and renders the first page', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => page(['a', 'b'], null) }) as unknown as typeof fetch
    render(<Feed />)
    await waitFor(() => expect(screen.getByText('t-a')).toBeInTheDocument())
    expect(screen.getByText('t-b')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /加载更多|更多/ })).toBeNull()
  })

  it('shows load-more and appends the next page via cursor', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: true, json: async () => page(['a'], 'CUR1') })
      .mockResolvedValueOnce({ ok: true, json: async () => page(['b'], null) })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<Feed />)
    await waitFor(() => expect(screen.getByText('t-a')).toBeInTheDocument())

    const more = screen.getByRole('button', { name: /加载更多|更多/ })
    fireEvent.click(more)

    await waitFor(() => expect(screen.getByText('t-b')).toBeInTheDocument())
    expect(screen.getByText('t-a')).toBeInTheDocument()
    const secondUrl = fetchMock.mock.calls[1][0] as string
    expect(secondUrl).toContain('cursor=CUR1')
  })

  it('submitting the search box refetches with q and resets the list', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: true, json: async () => page(['a', 'b'], null) })
      .mockResolvedValueOnce({ ok: true, json: async () => page(['s1'], null) })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<Feed />)
    await waitFor(() => expect(screen.getByText('t-a')).toBeInTheDocument())

    fireEvent.change(screen.getByPlaceholderText(/搜索/), { target: { value: 'OpenAI' } })
    fireEvent.submit(screen.getByRole('search'))

    await waitFor(() => expect(screen.getByText('t-s1')).toBeInTheDocument())
    expect(screen.queryByText('t-a')).toBeNull()
    const searchUrl = fetchMock.mock.calls[1][0] as string
    expect(searchUrl).toContain('q=OpenAI')
  })

  it('shows an error message when the fetch fails', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 }) as unknown as typeof fetch
    render(<Feed />)
    await waitFor(() => expect(screen.getByText(/加载失败|出错|错误/)).toBeInTheDocument())
  })
})
