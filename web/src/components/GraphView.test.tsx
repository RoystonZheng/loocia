import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { CloudWord } from '../cloudLayout'
import { GraphView } from './GraphView'

vi.mock('../cloudLayout', () => ({
  layoutCloud: async (words: CloudWord[]) =>
    words.map((w, i) => ({ ...w, x: i * 60 - 100, y: 0 })),
}))

const cloudPayload = {
  terms: [
    { term: 'OpenAI', kind: 'entity', count: 42 },
    { term: '开源', kind: 'topic', count: 7 },
  ],
}
const termPayload = {
  term: 'OpenAI', kind: 'entity', count: 42,
  neighbors: [{ term: '推理模型', kind: 'topic', weight: 3 }],
  items: [{ id: 'i1', title: '大新闻', url: 'https://x/i1', permalink: '/items/i1', source: 'S', selected: true }],
}

function mockFetch() {
  return vi.fn().mockImplementation((url: string) => {
    if (String(url).includes('/graph/term/')) {
      return Promise.resolve({ ok: true, json: async () => termPayload })
    }
    return Promise.resolve({ ok: true, json: async () => cloudPayload })
  }) as unknown as typeof fetch
}

describe('GraphView', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('renders the cloud words after loading', async () => {
    globalThis.fetch = mockFetch()
    render(<GraphView />)
    await waitFor(() => expect(screen.getByText('OpenAI')).toBeInTheDocument())
    expect(screen.getByText('开源')).toBeInTheDocument()
    expect(globalThis.fetch).toHaveBeenCalledWith('/api/public/graph/cloud?window=7d')
  })

  it('clicking a word opens the term panel', async () => {
    globalThis.fetch = mockFetch()
    render(<GraphView />)
    await waitFor(() => expect(screen.getByText('OpenAI')).toBeInTheDocument())
    fireEvent.click(screen.getByText('OpenAI'))
    await waitFor(() => expect(screen.getByText('推理模型')).toBeInTheDocument())
    expect(globalThis.fetch).toHaveBeenCalledWith(`/api/public/graph/term/${encodeURIComponent('OpenAI')}?window=7d`)
  })

  it('switching the window refetches the cloud', async () => {
    globalThis.fetch = mockFetch()
    render(<GraphView />)
    await waitFor(() => expect(screen.getByText('OpenAI')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: '30 天' }))
    await waitFor(() =>
      expect(globalThis.fetch).toHaveBeenCalledWith('/api/public/graph/cloud?window=30d'))
  })

  it('shows the empty state for an empty window', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ terms: [] }) }) as unknown as typeof fetch
    render(<GraphView />)
    await waitFor(() => expect(screen.getByText(/该时间段暂无数据/)).toBeInTheDocument())
  })

  it('shows the error state when the cloud fetch fails', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 }) as unknown as typeof fetch
    render(<GraphView />)
    await waitFor(() => expect(screen.getByText(/加载失败/)).toBeInTheDocument())
  })
})
