import { render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { HotTopics } from './HotTopics'

describe('HotTopics', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('renders topics with source-count badges', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ count: 2, items: [
        { id: 'h1', title: '大新闻', url: 'https://x/h1', permalink: '/items/h1', source: 'A', sourceCount: 4, sourceNames: ['A', 'B'], latestAt: '2026-05-07T10:00:00Z' },
        { id: 'h2', title: '次热', url: 'https://x/h2', permalink: '/items/h2', source: 'C', sourceCount: 2, sourceNames: ['C'], latestAt: '2026-05-07T09:00:00Z' },
      ] }),
    }) as unknown as typeof fetch
    render(<HotTopics />)
    await waitFor(() => expect(screen.getByText('当前热点')).toBeInTheDocument())
    expect(screen.getByRole('link', { name: '大新闻' })).toHaveAttribute('href', '/items/h1')
    expect(screen.getByText('4 个来源')).toBeInTheDocument()
    expect(screen.getByText('次热')).toBeInTheDocument()
  })

  it('renders nothing when empty', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ count: 0, items: [] }) }) as unknown as typeof fetch
    const { container } = render(<HotTopics />)
    await waitFor(() => expect(container.innerHTML).toBe(''))
  })

  it('renders nothing on error', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 }) as unknown as typeof fetch
    const { container } = render(<HotTopics />)
    await waitFor(() => expect(container.innerHTML).toBe(''))
  })
})
