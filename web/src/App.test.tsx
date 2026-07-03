import { render, screen, waitFor } from '@testing-library/react'
import { vi } from 'vitest'
import App from './App'

it('renders the api version', async () => {
  globalThis.fetch = vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({ apiVersion: '1.1.0', skillVersion: '0.1.0', updatedAt: '2026-07-03', changelogUrl: '/changelog', recentChanges: [] }),
  }) as unknown as typeof fetch
  render(<App />)
  await waitFor(() => expect(screen.getByText(/1\.1\.0/)).toBeInTheDocument())
})
