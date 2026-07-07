import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { DailyView } from './DailyView'

const report05 = {
  date: '2026-05-07', generatedAt: '2026-05-08T01:00:00Z',
  windowStart: '2026-05-07T00:00:00Z', windowEnd: '2026-05-08T00:00:00Z',
  lead: { title: '今日AI看点', leadParagraph: '模型层面动作频繁。' },
  sections: [{
    label: '模型发布/更新',
    items: [{ title: '条目一', summary: '摘要一', sourceUrl: 'https://x/1', sourceName: 'SrcA', permalink: '/items/1' }],
  }],
  flashes: [{ title: '快讯一', sourceName: 'SrcB', sourceUrl: 'https://x/2', publishedAt: '2026-05-07T04:00:00Z', permalink: '/items/2' }],
}
const report06 = { ...report05, date: '2026-05-06', lead: { title: '前一天看点', leadParagraph: '前一天内容摘要。' }, sections: [], flashes: [] }

const list = { count: 2, items: [
  { date: '2026-05-07', generatedAt: '2026-05-08T01:00:00Z', leadTitle: '今日AI看点', leadParagraph: '模型层面动作频繁。' },
  { date: '2026-05-06', generatedAt: '2026-05-07T01:00:00Z', leadTitle: '前一天看点', leadParagraph: '前一天内容摘要。' },
] }

function mockFetch(indexOk = true) {
  return vi.fn(async (url: string) => {
    const u = String(url)
    if (u.endsWith('/dailies')) return { ok: indexOk, status: indexOk ? 200 : 500, json: async () => list } as Response
    if (u.endsWith('/2026-05-07')) return { ok: true, status: 200, json: async () => report05 } as Response
    if (u.endsWith('/2026-05-06')) return { ok: true, status: 200, json: async () => report06 } as Response
    return { ok: false, status: 404 } as Response
  })
}

describe('DailyView', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('renders the latest report as a magazine', async () => {
    globalThis.fetch = mockFetch() as unknown as typeof fetch
    render(<DailyView />)
    await waitFor(() => expect(screen.getByText('模型层面动作频繁。')).toBeInTheDocument())
    // section label appears in both 今日看点 and the full section
    expect(screen.getAllByText('模型发布/更新').length).toBeGreaterThan(0)
    expect(screen.getByText('摘要一')).toBeInTheDocument() // unique to the full section
    expect(screen.getByText(/VOL\.2026\.05\.07/)).toBeInTheDocument()
    // archive lists both days
    expect(screen.getByText('前一天看点')).toBeInTheDocument()
  })

  it('navigates via the date archive', async () => {
    globalThis.fetch = mockFetch() as unknown as typeof fetch
    render(<DailyView />)
    await waitFor(() => expect(screen.getByText('模型层面动作频繁。')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: /前一天看点/ }))
    await waitFor(() => expect(screen.getByText('前一天内容摘要。')).toBeInTheDocument())
    expect(screen.getByText(/VOL\.2026\.05\.06/)).toBeInTheDocument()
  })

  it('shows empty state when the archive is empty', async () => {
    globalThis.fetch = vi.fn(async () => ({ ok: true, status: 200, json: async () => ({ count: 0, items: [] }) })) as unknown as typeof fetch
    render(<DailyView />)
    await waitFor(() => expect(screen.getByText(/暂无日报/)).toBeInTheDocument())
  })

  it('shows error state when the index fails', async () => {
    globalThis.fetch = mockFetch(false) as unknown as typeof fetch
    render(<DailyView />)
    await waitFor(() => expect(screen.getByText(/加载失败/)).toBeInTheDocument())
  })
})
