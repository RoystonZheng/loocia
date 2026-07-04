import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { vi } from 'vitest'
import App from './App'

function mockAll() {
  globalThis.fetch = vi.fn().mockImplementation((url: string) => {
    const u = String(url)
    if (u.includes('/api/public/version')) {
      return Promise.resolve({ ok: true, json: async () => ({ apiVersion: '1.1.0', skillVersion: '0.1.0', updatedAt: '2026-07-03', changelogUrl: '/changelog', recentChanges: [] }) })
    }
    if (u.includes('/api/public/daily')) {
      return Promise.resolve({ ok: true, json: async () => ({ date: '2026-05-07', generatedAt: '2026-05-08T01:00:00Z', windowStart: '2026-05-07T00:00:00Z', windowEnd: '2026-05-08T00:00:00Z', lead: { title: '日报导语', leadParagraph: '段落' }, sections: [], flashes: [] }) })
    }
    return Promise.resolve({ ok: true, json: async () => ({ count: 1, hasNext: false, nextCursor: null, items: [{ id: 'a', title: '标题A', url: 'https://x/a', permalink: '/items/a', source: 'S', selected: true }] }) })
  }) as unknown as typeof fetch
}

it('renders the feed by default and switches to the daily view', async () => {
  mockAll()
  render(<App />)
  await waitFor(() => expect(screen.getByText('标题A')).toBeInTheDocument())

  fireEvent.click(screen.getByRole('button', { name: '日报' }))
  await waitFor(() => expect(screen.getByText('日报导语')).toBeInTheDocument())
  expect(screen.queryByText('标题A')).toBeNull()

  fireEvent.click(screen.getByRole('button', { name: '资讯流' }))
  await waitFor(() => expect(screen.getByText('标题A')).toBeInTheDocument())
})
