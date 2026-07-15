import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { CloudWord } from '../cloudLayout'
import { WordCloud } from './WordCloud'

vi.mock('../cloudLayout', () => ({
  layoutCloud: async (words: CloudWord[]) =>
    words.map((w, i) => ({ ...w, x: i * 60 - 100, y: 0 })),
}))

const cloud = { terms: [{ term: '大模型', kind: 'topic', count: 9 }] }
const term = {
  term: '大模型', kind: 'topic', count: 9,
  neighbors: [{ term: '推理', kind: 'topic', weight: 2 }],
  items: [{ id: 'i1', title: '相关新闻', url: 'https://x/i1', permalink: '/items/i1', source: 'S', selected: true }],
}

function mockFetch() {
  return vi.fn().mockImplementation((url: string) =>
    Promise.resolve({
      ok: true,
      json: async () => (String(url).includes('/graph/term/') ? term : cloud),
    }),
  ) as unknown as typeof fetch
}

describe('WordCloud per-day mode', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('fetches the given day and hides the window selector', async () => {
    globalThis.fetch = mockFetch()
    render(<WordCloud date="2026-07-14" />)
    await waitFor(() => expect(screen.getByText('大模型')).toBeInTheDocument())
    expect(globalThis.fetch).toHaveBeenCalledWith('/api/public/graph/cloud?date=2026-07-14')
    // no 7天/30天/全部 selector in date mode
    expect(screen.queryByRole('button', { name: '7 天' })).toBeNull()
  })

  it('drilling into a word stays scoped to the day', async () => {
    globalThis.fetch = mockFetch()
    render(<WordCloud date="2026-07-14" />)
    await waitFor(() => expect(screen.getByText('大模型')).toBeInTheDocument())
    fireEvent.click(screen.getByText('大模型'))
    await waitFor(() =>
      expect(globalThis.fetch).toHaveBeenCalledWith(
        `/api/public/graph/term/${encodeURIComponent('大模型')}?date=2026-07-14`))
  })

  it('window mode (no date) shows the selector', async () => {
    globalThis.fetch = mockFetch()
    render(<WordCloud />)
    await waitFor(() => expect(screen.getByText('大模型')).toBeInTheDocument())
    expect(screen.getByRole('button', { name: '7 天' })).toBeInTheDocument()
    expect(globalThis.fetch).toHaveBeenCalledWith('/api/public/graph/cloud?window=7d')
  })
})
