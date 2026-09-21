import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { ItemCard } from './ItemCard'
import type { PublicItem } from '../api/items'

const base: PublicItem = {
  id: 'a', title: '模型 X 发布', url: 'https://ex.com/a', permalink: '/items/a',
  source: 'OpenAI Blog', publishedAt: '2026-05-07T04:00:00Z',
  summary: '这是一段中文摘要。', category: 'ai-models', score: 4, selected: true,
}

describe('ItemCard', () => {
  it('renders title, source, category label, score, summary', () => {
    render(<ItemCard item={base} />)
    expect(screen.getByText('模型 X 发布')).toBeInTheDocument()
    expect(screen.getByText('OpenAI Blog')).toBeInTheDocument()
    expect(screen.getByText('模型发布/更新')).toBeInTheDocument()
    expect(screen.getByText('A')).toBeInTheDocument()
    expect(screen.getByText('这是一段中文摘要。')).toBeInTheDocument()
  })

  it('hides the score badge on non-selected items', () => {
    // The score is a 精选 quality signal; a low score on a 全部 item is noise.
    render(<ItemCard item={{ ...base, selected: false, score: 1 }} />)
    expect(document.querySelector('.badge-score')).toBeNull()
    expect(screen.queryByText('D')).toBeNull()
  })

  it('shows the score badge on selected items', () => {
    render(<ItemCard item={{ ...base, selected: true, score: 5 }} />)
    expect(document.querySelector('.badge-score')).not.toBeNull()
    expect(screen.getByText('S')).toBeInTheDocument()
    expect(document.querySelector('.badge-score')!.getAttribute('data-tier')).toBe('S')
  })

  it('marks AIHOT-sourced items without replacing the original source', () => {
    render(<ItemCard item={{ ...base, source: 'Preferred Networks', sourceKind: 'aihot', selected: false }} />)
    expect(screen.getByText('Preferred Networks')).toBeInTheDocument()
    expect(screen.getByText('AIHOT补漏')).toBeInTheDocument()
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
    // no image when imageUrl absent
    expect(document.querySelector('img.card-image')).toBeNull()
  })

  it('renders a thumbnail when imageUrl is present', () => {
    render(<ItemCard item={{ ...base, imageUrl: 'https://cdn.ex.com/hero.jpg' }} />)
    const img = document.querySelector('img.card-image') as HTMLImageElement | null
    expect(img).not.toBeNull()
    expect(img!.getAttribute('src')).toBe('https://cdn.ex.com/hero.jpg')
  })

  it('shows a play overlay over the thumbnail when videoUrl is present', () => {
    render(<ItemCard item={{ ...base, imageUrl: 'https://cdn.ex.com/hero.jpg', videoUrl: 'https://cdn.ex.com/clip.mp4' }} />)
    expect(document.querySelector('img.card-image')).not.toBeNull()
    expect(document.querySelector('.card-play')).not.toBeNull()
    // The media wrapper links to the detail page where the video plays.
    const media = document.querySelector('.card-media') as HTMLAnchorElement | null
    expect(media).not.toBeNull()
    expect(media!.getAttribute('href')).toBe('/items/a')
  })

  it('shows a bare video affordance when video has no thumbnail', () => {
    render(<ItemCard item={{ ...base, videoUrl: 'https://cdn.ex.com/clip.mp4' }} />)
    expect(document.querySelector('img.card-image')).toBeNull()
    expect(document.querySelector('.card-media-bare')).not.toBeNull()
    expect(document.querySelector('.card-play')).not.toBeNull()
  })

  it('shows no play overlay when there is no video', () => {
    render(<ItemCard item={{ ...base, imageUrl: 'https://cdn.ex.com/hero.jpg' }} />)
    expect(document.querySelector('.card-play')).toBeNull()
  })
})
