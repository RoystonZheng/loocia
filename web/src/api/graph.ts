import type { PublicItem } from './items'

export type GraphWindow = '7d' | '30d' | 'all'

export interface CloudTerm {
  term: string
  kind: 'entity' | 'topic'
  count: number
}

export interface CloudResponse {
  terms: CloudTerm[]
}

export interface GraphNeighbor {
  term: string
  kind: 'entity' | 'topic'
  weight: number
}

export interface TermResponse {
  term: string
  kind: string
  count: number
  neighbors: GraphNeighbor[]
  items: PublicItem[]
}

// scopeQuery builds the time filter: a specific Beijing day (date) takes
// precedence over the rolling window, so the daily report can show that day's
// own cloud.
function scopeQuery(window: GraphWindow, date?: string): string {
  return date ? `date=${encodeURIComponent(date)}` : `window=${window}`
}

export async function fetchCloud(window: GraphWindow, date?: string): Promise<CloudResponse> {
  const res = await fetch(`/api/public/graph/cloud?${scopeQuery(window, date)}`)
  if (!res.ok) throw new Error(`graph cloud fetch failed: ${res.status}`)
  return (await res.json()) as CloudResponse
}

export async function fetchTerm(term: string, window: GraphWindow, date?: string): Promise<TermResponse> {
  const res = await fetch(`/api/public/graph/term/${encodeURIComponent(term)}?${scopeQuery(window, date)}`)
  if (!res.ok) throw new Error(`graph term fetch failed: ${res.status}`)
  return (await res.json()) as TermResponse
}
