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

it('renders 精选 by default and switches to the daily view', async () => {
  mockAll()
  render(<App />)
  await waitFor(() => expect(screen.getByText('标题A')).toBeInTheDocument())

  // hot-topics strip renders above the 精选 feed
  await waitFor(() => expect(screen.getByText(/当前热点/)).toBeInTheDocument())
  expect(screen.getByText('热点一')).toBeInTheDocument()

  fireEvent.click(screen.getByRole('button', { name: 'AI 日报' }))
  // the magazine's archive shows the day's lead title
  await waitFor(() => expect(screen.getByText('日报导语')).toBeInTheDocument())
  expect(screen.queryByText('标题A')).toBeNull()

  fireEvent.click(screen.getByRole('button', { name: '精选' }))
  await waitFor(() => expect(screen.getByText('标题A')).toBeInTheDocument())
})

it('switches to the graph view', async () => {
  mockAll()
  render(<App />)
  await waitFor(() => expect(screen.getByText('标题A')).toBeInTheDocument())
  fireEvent.click(screen.getByRole('button', { name: '图谱' }))
  await waitFor(() => expect(screen.getByText('词云词')).toBeInTheDocument())
  expect(screen.queryByText('标题A')).toBeNull()
})
