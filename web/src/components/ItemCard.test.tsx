import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { ItemCard } from './ItemCard'
import type { PublicItem } from '../api/items'

const base: PublicItem = {
  id: 'a', title: '模型 X 发布', url: 'https://ex.com/a', permalink: '/items/a',
  source: 'OpenAI Blog', publishedAt: '2026-05-07T04:00:00Z',
  summary: '这是一段中文摘要。', category: 'ai-models', score: 88, selected: true,
}

describe('ItemCard', () => {
  it('renders title, source, category label, score, summary', () => {
    render(<ItemCard item={base} />)
    expect(screen.getByText('模型 X 发布')).toBeInTheDocument()
    expect(screen.getByText('OpenAI Blog')).toBeInTheDocument()
    expect(screen.getByText('模型发布/更新')).toBeInTheDocument()
    expect(screen.getByText(/88/)).toBeInTheDocument()
    expect(screen.getByText('这是一段中文摘要。')).toBeInTheDocument()
  })

  it('renders Beijing time', () => {
    render(<ItemCard item={base} />)
    expect(screen.getByText(/2026-05-07 12:00/)).toBeInTheDocument()
  })

  it('links the title to the permalink', () => {
    render(<ItemCard item={base} />)
    const link = screen.getByRole('link', { name: /模型 X 发布/ })
    expect(link).toHaveAttribute('href', '/items/a')
  })

  it('omits optional fields cleanly', () => {
    const minimal: PublicItem = {
      id: 'b', title: 'T', url: 'https://x/b', permalink: '/items/b', source: 'S', selected: false,
    }
    render(<ItemCard item={minimal} />)
    expect(screen.getByText('T')).toBeInTheDocument()
    expect(screen.queryByText(/undefined/)).toBeNull()
  })
})
