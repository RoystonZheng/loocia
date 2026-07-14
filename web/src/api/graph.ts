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

export async function fetchCloud(window: GraphWindow): Promise<CloudResponse> {
  const res = await fetch(`/api/public/graph/cloud?window=${window}`)
  if (!res.ok) throw new Error(`graph cloud fetch failed: ${res.status}`)
  return (await res.json()) as CloudResponse
}

export async function fetchTerm(term: string, window: GraphWindow): Promise<TermResponse> {
  const res = await fetch(`/api/public/graph/term/${encodeURIComponent(term)}?window=${window}`)
  if (!res.ok) throw new Error(`graph term fetch failed: ${res.status}`)
  return (await res.json()) as TermResponse
}
