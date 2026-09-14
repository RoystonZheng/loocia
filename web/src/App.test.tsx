import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { vi } from 'vitest'
import App from './App'

vi.mock('./cloudLayout', () => ({
  layoutCloud: async (words: { text: string; size: number; kind: string }[]) =>
    words.map((w, i) => ({ ...w, x: i * 60 - 100, y: 0 })),
}))

const report = {
  date: '2026-05-07', generatedAt: '2026-05-08T01:00:00Z',
  windowStart: '2026-05-07T00:00:00Z', windowEnd: '2026-05-08T00:00:00Z',
  lead: { title: '日报导语', leadParagraph: '这是导语段落。' },
  sections: [], flashes: [],
}

function mockAll() {
  globalThis.fetch = vi.fn().mockImplementation((url: string) => {
    const u = String(url)
    if (u.includes('/api/public/graph/cloud')) {
      return Promise.resolve({ ok: true, json: async () => ({ terms: [{ term: '词云词', kind: 'topic', count: 3 }] }) })
    }
    if (u.includes('/api/public/hot-topics')) {
      return Promise.resolve({ ok: true, json: async () => ({ count: 1, items: [{ id: 'h1', title: '热点一', url: 'https://x/h1', permalink: '/items/h1', source: 'S', sourceCount: 3, sourceNames: ['S'], latestAt: '2026-05-07T10:00:00Z' }] }) })
    }
    if (u.includes('/api/public/version')) {
      return Promise.resolve({ ok: true, json: async () => ({ apiVersion: '1.1.0', skillVersion: '0.1.0', updatedAt: '2026-07-03', changelogUrl: '/changelog', recentChanges: [] }) })
    }
    if (u.includes('/api/public/dailies')) {
      return Promise.resolve({ ok: true, json: async () => ({ count: 1, items: [{ date: '2026-05-07', generatedAt: '2026-05-08T01:00:00Z', leadTitle: '日报导语', leadParagraph: '这是导语段落。' }] }) })
    }
    if (u.includes('/api/public/daily/')) {
      return Promise.resolve({ ok: true, json: async () => report })
    }
    return Promise.resolve({ ok: true, json: async () => ({ count: 1, hasNext: false, nextCursor: null, items: [{ id: 'a', title: '标题A', url: 'https://x/a', permalink: '/items/a', source: 'S', selected: true, publishedAt: '2026-05-07T10:00:00Z' }] }) })
  }) as unknown as typeof fetch
}

it('renders the daily home with the word cloud by default, and switches views', async () => {
  mockAll()
  render(<App />)
  // home = the daily report (日报导语) with its per-day 今日热词 cloud (词云词);
  // the feed is not shown
  await waitFor(() => expect(screen.getByText('今日热词')).toBeInTheDocument())
  expect(screen.getByText('日报导语')).toBeInTheDocument()
  await waitFor(() => expect(screen.getByText('词云词')).toBeInTheDocument())
  expect(screen.queryByText('标题A')).toBeNull()

  // 精选 → feed + hot-topics strip
  fireEvent.click(screen.getByRole('button', { name: '精选资讯' }))
  await waitFor(() => expect(screen.getByText('标题A')).toBeInTheDocument())
  expect(screen.getByText(/当前热点/)).toBeInTheDocument()
  expect(screen.getByText('热点一')).toBeInTheDocument()

  // back to the daily home
  fireEvent.click(screen.getByRole('button', { name: 'AI 日报' }))
  await waitFor(() => expect(screen.getByText('日报导语')).toBeInTheDocument())
  expect(screen.queryByText('标题A')).toBeNull()
})

it('switches to the standalone graph view', async () => {
  mockAll()
  render(<App />)
  await waitFor(() => expect(screen.getByText('日报导语')).toBeInTheDocument())
  fireEvent.click(screen.getByRole('button', { name: '话题图谱' }))
  // the graph page has its own subtitle, distinct from the home hero
  await waitFor(() => expect(screen.getByText(/词云看热度/)).toBeInTheDocument())
  expect(screen.queryByText('日报导语')).toBeNull()
})

it('deep-links to the 精选 feed via location hash', async () => {
  mockAll()
  window.location.hash = '#selected'
  try {
    render(<App />)
    await waitFor(() => expect(screen.getByText('标题A')).toBeInTheDocument())
  } finally {
    window.location.hash = ''
  }
})
