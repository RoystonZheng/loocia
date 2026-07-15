import { describe, it, expect } from 'vitest'
import { layoutGraph } from './graphLayout'
import { labelHalfWidth } from './ink'

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

describe('layoutGraph seeded', () => {
  it('keeps seeded nodes near their previous positions', () => {
    const nodes = [
      { id: 'focus', r: 26 },
      { id: 'a', r: 14 },
      { id: 'b', r: 10 },
    ]
    const edges = [
      { source: 'focus', target: 'a', width: 3 },
      { source: 'focus', target: 'b', width: 1 },
    ]
    const first = layoutGraph(nodes, edges, 400, 300)
    const seed = new Map(first.nodes.map((n) => [n.id, { x: n.x, y: n.y }]))
    const second = layoutGraph(nodes, edges, 400, 300, seed)
    for (const n of second.nodes) {
      const prev = seed.get(n.id)!
      const dist = Math.hypot(n.x - prev.x, n.y - prev.y)
      expect(dist).toBeLessThan(40) // 收敛过的种子不应大幅漂移
    }
  })

  it('clamps within label-aware margins', () => {
    const nodes = [{ id: '很长很长很长的一个标签词', r: 8 }]
    const { nodes: placed } = layoutGraph(nodes, [], 200, 120)
    const m = Math.max(8, labelHalfWidth('很长很长很长的一个标签词'))
    expect(placed[0].x).toBeGreaterThanOrEqual(m)
    expect(placed[0].x).toBeLessThanOrEqual(200 - m)
    expect(placed[0].y).toBeLessThanOrEqual(120 - (8 + 16)) // 下方留标签位
  })
})
