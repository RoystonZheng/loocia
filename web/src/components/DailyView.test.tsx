import { render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { DailyView } from './DailyView'

const report = {
  date: '2026-05-07', generatedAt: '2026-05-08T01:00:00Z',
  windowStart: '2026-05-07T00:00:00Z', windowEnd: '2026-05-08T00:00:00Z',
  lead: { title: '今日AI看点', leadParagraph: '模型层面动作频繁。' },
  sections: [{
    label: '模型发布/更新',
    items: [{ title: '条目一', summary: '摘要一', sourceUrl: 'https://x/1', sourceName: 'SrcA', permalink: '/items/1' }],
  }],
  flashes: [{ title: '快讯一', sourceName: 'SrcB', sourceUrl: 'https://x/2', publishedAt: '2026-05-07T04:00:00Z', permalink: '/items/2' }],
}

describe('DailyView', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('renders lead, sections, flashes', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => report }) as unknown as typeof fetch
    render(<DailyView />)
    await waitFor(() => expect(screen.getByText('今日AI看点')).toBeInTheDocument())
    expect(screen.getByText('模型层面动作频繁。')).toBeInTheDocument()
    expect(screen.getByText('模型发布/更新')).toBeInTheDocument()
    expect(screen.getByText('条目一')).toBeInTheDocument()
    expect(screen.getByText('摘要一')).toBeInTheDocument()
    expect(screen.getByText('快讯一')).toBeInTheDocument()
    // item links to permalink
    expect(screen.getByRole('link', { name: '条目一' })).toHaveAttribute('href', '/items/1')
    // date shown (header line; the flash timestamp also contains the date, so match the header specifically)
    expect(screen.getByText(/2026-05-07 · 生成于/)).toBeInTheDocument()
  })

  it('shows empty state on 404', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 404 }) as unknown as typeof fetch
    render(<DailyView />)
    await waitFor(() => expect(screen.getByText(/暂无日报/)).toBeInTheDocument())
  })

  it('shows error state on failure', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 }) as unknown as typeof fetch
    render(<DailyView />)
    await waitFor(() => expect(screen.getByText(/加载失败/)).toBeInTheDocument())
  })
})
