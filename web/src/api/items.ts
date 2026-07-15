export interface PublicItem {
  id: string
  title: string
  title_en?: string
  url: string
  permalink: string
  source: string
  publishedAt?: string
  summary?: string
  imageUrl?: string
  videoUrl?: string
  category?: string
  score?: number
  selected: boolean
}

export interface ItemList {
  count: number
  hasNext: boolean
  nextCursor: string | null
  items: PublicItem[]
}

export interface ListQuery {
  mode?: 'selected' | 'all'
  category?: string
  sourceKind?: 'mp' | 'rss'
  q?: string
  take?: number
  cursor?: string
}

export async function fetchItems(query: ListQuery): Promise<ItemList> {
  const params = new URLSearchParams()
  if (query.mode) params.set('mode', query.mode)
  if (query.category) params.set('category', query.category)
  if (query.sourceKind) params.set('source_kind', query.sourceKind)
  if (query.q) params.set('q', query.q)
  if (query.take != null) params.set('take', String(query.take))
  if (query.cursor) params.set('cursor', query.cursor)

  const res = await fetch(`/api/public/items?${params.toString()}`)
  if (!res.ok) throw new Error(`items fetch failed: ${res.status}`)
  return (await res.json()) as ItemList
}
