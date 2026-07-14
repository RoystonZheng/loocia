import { describe, it, expect } from 'vitest'
import { layoutGraph } from './graphLayout'

describe('layoutGraph', () => {
  it('positions every node with finite in-bounds coordinates', () => {
    const nodes = [
      { id: 'focus', r: 26 },
      { id: 'a', r: 14 },
      { id: 'b', r: 10 },
    ]
    const edges = [
      { source: 'focus', target: 'a', width: 3 },
      { source: 'focus', target: 'b', width: 1 },
    ]
    const { nodes: placed, edges: lines } = layoutGraph(nodes, edges, 400, 300)
    expect(placed).toHaveLength(3)
    for (const n of placed) {
      expect(Number.isFinite(n.x)).toBe(true)
      expect(Number.isFinite(n.y)).toBe(true)
      expect(n.x).toBeGreaterThanOrEqual(n.r)
      expect(n.x).toBeLessThanOrEqual(400 - n.r)
      expect(n.y).toBeGreaterThanOrEqual(n.r)
      expect(n.y).toBeLessThanOrEqual(300 - n.r)
    }
    expect(lines).toHaveLength(2)
    for (const l of lines) {
      expect(Number.isFinite(l.x1)).toBe(true)
      expect(Number.isFinite(l.y2)).toBe(true)
    }
  })

  it('handles a single node without edges', () => {
    const { nodes: placed, edges: lines } = layoutGraph([{ id: 'only', r: 20 }], [], 200, 200)
    expect(placed).toHaveLength(1)
    expect(lines).toHaveLength(0)
  })
})
