import { render, screen, waitFor } from '@testing-library/react'
import { vi } from 'vitest'
import App from './App'

it('renders the header and the feed', async () => {
  // App mounts Feed, which fetches items on load; also version in the footer.
  globalThis.fetch = vi.fn().mockImplementation((url: string) => {
    if (String(url).includes('/api/public/version')) {
      return Promise.resolve({ ok: true, json: async () => ({ apiVersion: '1.1.0', skillVersion: '0.1.0', updatedAt: '2026-07-03', changelogUrl: '/changelog', recentChanges: [] }) })
    }
    return Promise.resolve({ ok: true, json: async () => ({ count: 1, hasNext: false, nextCursor: null, items: [{ id: 'a', title: '标题A', url: 'https://x/a', permalink: '/items/a', source: 'S', selected: true }] }) })
  }) as unknown as typeof fetch

  render(<App />)
  expect(screen.getByRole('heading', { name: /AI HOT/ })).toBeInTheDocument()
  await waitFor(() => expect(screen.getByText('标题A')).toBeInTheDocument())
})
