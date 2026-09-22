export interface PublicItem {
  id: string
  title: string
  title_en?: string
  url: string
  permalink: string
  source: string
  sourceKind?: 'rss' | 'html' | 'mp' | 'aihot'
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

export type SourceKind = 'rss' | 'html' | 'mp' | 'aihot'

export interface ListQuery {
  mode?: 'selected' | 'all'
  category?: string | string[]
  sourceKind?: SourceKind | SourceKind[]
  q?: string
  scoreMin?: number
  take?: number
  cursor?: string
}

export async function fetchItems(query: ListQuery): Promise<ItemList> {
  const params = new URLSearchParams()
  if (query.mode) params.set('mode', query.mode)
  for (const category of asArray(query.category)) params.append('category', category)
  for (const sourceKind of asArray(query.sourceKind)) params.append('source_kind', sourceKind)
  if (query.q) params.set('q', query.q)
  if (query.scoreMin != null) params.set('score_min', String(query.scoreMin))
  if (query.take != null) params.set('take', String(query.take))
  if (query.cursor) params.set('cursor', query.cursor)

  const res = await fetch(`/api/public/items?${params.toString()}`)
  if (!res.ok) throw new Error(`items fetch failed: ${res.status}`)
  return (await res.json()) as ItemList
}

function asArray<T>(value: T | T[] | undefined): T[] {
  return value == null ? [] : Array.isArray(value) ? value : [value]
}
