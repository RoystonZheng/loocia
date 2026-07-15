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

const termPayload2 = {
  term: '推理模型', kind: 'topic', count: 18,
  neighbors: [
    { term: 'OpenAI', kind: 'entity', weight: 9 },
    { term: '芯片', kind: 'topic', weight: 2 },
  ],
  items: [
    { id: 'i2', title: '另一条', url: 'https://x/i2', permalink: '/items/i2', source: 'S2', selected: false },
  ],
}

function mockTwoTerms() {
  globalThis.fetch = vi.fn().mockImplementation((url: string) => {
    const u = decodeURIComponent(String(url))
    if (u.includes('/graph/term/推理模型')) {
      return Promise.resolve({ ok: true, json: async () => termPayload2 })
    }
    return Promise.resolve({ ok: true, json: async () => termPayload })
  }) as unknown as typeof fetch
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
    // meta line: source + publishedAt formatted in Beijing time (08:00Z → 16:00 +08:00)
    expect(screen.getByText(/OpenAI Blog · 2026-07-10 16:00/)).toBeInTheDocument()
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

  it('keeps shared nodes mounted across a focus switch (smooth transition)', async () => {
    mockTwoTerms()
    const { rerender } = render(<TermPanel term="OpenAI" window="7d" onSelect={() => {}} />)
    await waitFor(() => expect(screen.getByText('推理模型')).toBeInTheDocument())
    const before = screen.getByText('OpenAI').closest('g')
    rerender(<TermPanel term="推理模型" window="7d" onSelect={() => {}} />)
    await waitFor(() => expect(screen.getByText('芯片')).toBeInTheDocument())
    // OpenAI 在两轮里都存在（先焦点后邻居）：DOM 节点必须是同一个（没被重建）
    expect(screen.getByText('OpenAI').closest('g')).toBe(before)
  })

  it('fades out and removes exiting nodes after the transition', async () => {
    vi.useFakeTimers()
    try {
      mockTwoTerms()
      const { rerender } = render(<TermPanel term="OpenAI" window="7d" onSelect={() => {}} />)
      await vi.waitFor(() => expect(screen.getByText('开源')).toBeInTheDocument())
      rerender(<TermPanel term="推理模型" window="7d" onSelect={() => {}} />)
      await vi.waitFor(() => expect(screen.getByText('芯片')).toBeInTheDocument())
      // 「开源」不在新邻居里：先带 exiting 类（保留自己的墨阶），计时器走完后从 DOM 消失
      expect(screen.getByText('开源').closest('g')!.className.baseVal).toContain('exiting')
      expect(screen.getByText('开源').closest('g')!.className.baseVal).toContain('gv-ink-t')
      vi.advanceTimersByTime(500)
      expect(screen.queryByText('开源')).toBeNull()
    } finally {
      vi.useRealTimers()
    }
  })

  it('dims非 hover 的节点', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => termPayload }) as unknown as typeof fetch
    render(<TermPanel term="OpenAI" window="7d" onSelect={() => {}} />)
    await waitFor(() => expect(screen.getByText('推理模型')).toBeInTheDocument())
    fireEvent.mouseEnter(screen.getByText('推理模型').closest('g')!)
    expect(screen.getByText('开源').closest('g')!.className.baseVal).toContain('dim')
    expect(screen.getByText('推理模型').closest('g')!.className.baseVal).not.toContain('dim')
    fireEvent.mouseLeave(screen.getByText('推理模型').closest('g')!)
    expect(screen.getByText('开源').closest('g')!.className.baseVal).not.toContain('dim')
  })

  it('hover 不改变节点几何（布局在数据不变时冻结）', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => termPayload }) as unknown as typeof fetch
    render(<TermPanel term="OpenAI" window="7d" onSelect={() => {}} />)
    await waitFor(() => expect(screen.getByText('推理模型')).toBeInTheDocument())
    const node = screen.getByText('开源').closest('g')!
    const before = node.getAttribute('transform')
    expect(before).toBeTruthy()
    fireEvent.mouseEnter(screen.getByText('推理模型').closest('g')!)
    expect(node.getAttribute('transform')).toBe(before)
    fireEvent.mouseLeave(screen.getByText('推理模型').closest('g')!)
    expect(node.getAttribute('transform')).toBe(before)
  })
})
