import {
  forceCenter,
  forceCollide,
  forceLink,
  forceManyBody,
  forceSimulation,
  type SimulationLinkDatum,
  type SimulationNodeDatum,
} from 'd3-force'
import { labelHalfWidth } from './ink'

export interface GraphNode {
  id: string
  r: number
}

export interface GraphEdge {
  source: string
  target: string
  width: number
}

export interface PlacedNode extends GraphNode {
  x: number
  y: number
}

export interface PlacedEdge {
  x1: number
  y1: number
  x2: number
  y2: number
  width: number
}

type SimNode = GraphNode & SimulationNodeDatum

// layoutGraph runs a force simulation synchronously (fixed ticks, no timers) —
// deterministic enough for a static render and jsdom-safe (pure math, no DOM).
// When `seed` carries previous positions, matching nodes start there with a
// lowered alpha so the new layout stays visually continuous (smooth-transition
// animation relies on this).
export function layoutGraph(
  nodes: GraphNode[],
  edges: GraphEdge[],
  width: number,
  height: number,
  seed?: Map<string, { x: number; y: number }>,
): { nodes: PlacedNode[]; edges: PlacedEdge[] } {
  const ns: SimNode[] = nodes.map((n) => {
    const s = seed?.get(n.id)
    return s ? { ...n, x: s.x, y: s.y } : { ...n }
  })
  const ls: (SimulationLinkDatum<SimNode> & { width: number })[] = edges.map((e) => ({ ...e }))

  const sim = forceSimulation(ns)
    .force('link', forceLink(ls).id((d: SimulationNodeDatum) => (d as SimNode).id).distance(90))
    .force('charge', forceManyBody().strength(-160))
    .force('center', forceCenter(width / 2, height / 2))
    .force('collide', forceCollide<SimNode>().radius((d) => Math.max(d.r, labelHalfWidth(d.id)) + 10))
    .stop()
  if (seed && seed.size > 0) sim.alpha(0.3) // 种子已接近平衡，别把它踢飞
  for (let i = 0; i < 200; i++) sim.tick()

  const marginX = (n: SimNode) => Math.max(n.r, labelHalfWidth(n.id))
  const clampX = (n: SimNode) => Math.min(Math.max(n.x ?? 0, marginX(n)), width - marginX(n))
  const clampY = (n: SimNode) => Math.min(Math.max(n.y ?? 0, n.r), height - (n.r + 16))
  const placed = ns.map((n) => ({ id: n.id, r: n.r, x: clampX(n), y: clampY(n) }))
  const byID = new Map(placed.map((n) => [n.id, n]))

  const lines: PlacedEdge[] = ls.map((l) => {
    // forceLink resolves source/target strings into node objects during tick
    const s = byID.get(typeof l.source === 'object' ? (l.source as SimNode).id : String(l.source))
    const t = byID.get(typeof l.target === 'object' ? (l.target as SimNode).id : String(l.target))
    return { x1: s?.x ?? 0, y1: s?.y ?? 0, x2: t?.x ?? 0, y2: t?.y ?? 0, width: l.width }
  })
  return { nodes: placed, edges: lines }
}
