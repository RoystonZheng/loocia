import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { TermPanel } from './TermPanel'

const termPayload = {
  term: 'OpenAI', kind: 'entity', count: 42,
  neighbors: [
    { term: '推理模型', kind: 'topic', weight: 18 },
    { term: '开源', kind: 'topic', weight: 5 },
  ],
  items: [
    { id: 'i1', title: '大新闻', url: 'https://x/i1', permalink: '/items/i1', source: 'OpenAI Blog', selected: true, publishedAt: '2026-07-10T08:00:00Z' },
  ],
}

describe('TermPanel', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('renders subgraph neighbors and related items', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => termPayload }) as unknown as typeof fetch
    render(<TermPanel term="OpenAI" window="7d" onSelect={() => {}} />)
    await waitFor(() => expect(screen.getByText('推理模型')).toBeInTheDocument())
    expect(globalThis.fetch).toHaveBeenCalledWith(`/api/public/graph/term/${encodeURIComponent('OpenAI')}?window=7d`)
    expect(screen.getByRole('link', { name: '大新闻' })).toHaveAttribute('href', '/items/i1')
    expect(screen.getByText(/42 条相关/)).toBeInTheDocument()
  })

  it('clicking a neighbor switches focus via onSelect', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => termPayload }) as unknown as typeof fetch
    const onSelect = vi.fn()
    render(<TermPanel term="OpenAI" window="7d" onSelect={onSelect} />)
    await waitFor(() => expect(screen.getByText('推理模型')).toBeInTheDocument())
    fireEvent.click(screen.getByText('推理模型'))
    expect(onSelect).toHaveBeenCalledWith('推理模型')
  })

  it('shows the empty state when the term has no data in the window', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ term: 'x', kind: '', count: 0, neighbors: [], items: [] }) }) as unknown as typeof fetch
    render(<TermPanel term="x" window="7d" onSelect={() => {}} />)
    await waitFor(() => expect(screen.getByText(/该时间段暂无数据/)).toBeInTheDocument())
  })

  it('shows the error state when the fetch fails', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 }) as unknown as typeof fetch
    render(<TermPanel term="x" window="7d" onSelect={() => {}} />)
    await waitFor(() => expect(screen.getByText(/加载失败/)).toBeInTheDocument())
  })
})
