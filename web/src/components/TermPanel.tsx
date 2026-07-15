import { useEffect, useRef, useState } from 'react'
import { flushSync } from 'react-dom'
import { fetchTerm, type GraphWindow, type TermResponse } from '../api/graph'
import { layoutGraph, type GraphEdge, type GraphNode, type PlacedNode } from '../graphLayout'
import { inkTier } from '../ink'
import { formatBeijingTime } from '../format'

const W = 420
const H = 320
const FOCUS_R = 26
const EXIT_MS = 400

interface Rendered extends PlacedNode {
  tier: 1 | 2 | 3 | 4
  isFocus: boolean
  entering: boolean
  exiting: boolean
}

// TermPanel is the drill-down under the cloud: an ink-styled co-occurrence
// subgraph with staged transitions (shared nodes slide, leavers fade out,
// newcomers fade in) + the term's newest items.
export function TermPanel({
  term, window: win, onSelect,
}: {
  term: string
  window: GraphWindow
  onSelect: (term: string) => void
}) {
  const [data, setData] = useState<TermResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [failed, setFailed] = useState(false)
  const [hovered, setHovered] = useState<string | null>(null)
  const [exiting, setExiting] = useState<Rendered[]>([])
  // positions survive data updates so the next layout can seed from them
  const positions = useRef(new Map<string, { x: number; y: number }>())
  // leavers computed during render (while `positions.current` still holds the
  // *previous* layout) get parked here; an effect flushes them into state right
  // after commit so we never call setState mid-render
  const pendingLeavers = useRef<Rendered[] | null>(null)

  useEffect(() => {
    let stale = false
    setLoading(true)
    setFailed(false)
    setHovered(null)
    fetchTerm(term, win)
      .then((d) => { if (!stale) { setData(d); setLoading(false) } })
      .catch(() => { if (!stale) { setFailed(true); setLoading(false) } })
    return () => { stale = true }
  }, [term, win])

  // flush leavers staged during the render that just committed
  useEffect(() => {
    const leavers = pendingLeavers.current
    if (leavers && leavers.length > 0) {
      pendingLeavers.current = null
      setExiting((old) => [...old, ...leavers])
    }
  }, [data])

  // exiting nodes leave the DOM after their fade-out completes
  useEffect(() => {
    if (exiting.length === 0) return
    const t = setTimeout(() => { flushSync(() => setExiting([])) }, EXIT_MS)
    return () => clearTimeout(t)
  }, [exiting])

  if (failed) return <div className="gv-panel gv-status">加载失败，稍后再试</div>
  if (!data) return <div className="gv-panel gv-status">{loading ? '加载中…' : '该时间段暂无数据'}</div>
  if (data.count === 0) return <div className="gv-panel gv-status">该时间段暂无数据</div>

  const maxW = Math.max(...data.neighbors.map((n) => n.weight), 1)
  const nodes: GraphNode[] = [
    { id: data.term, r: FOCUS_R },
    ...data.neighbors.map((n) => ({ id: n.term, r: 10 + 12 * (n.weight / maxW) })),
  ]
  const edges: GraphEdge[] = data.neighbors.map((n) => ({
    source: data.term, target: n.term, width: 1 + 3 * (n.weight / maxW),
  }))
  const prev = positions.current
  const g = layoutGraph(nodes, edges, W, H, prev)

  const tierOf = new Map<string, 1 | 2 | 3 | 4>([
    [data.term, 1],
    ...data.neighbors.map((n) => [n.term, inkTier(n.weight, maxW)] as const),
  ])
  const rendered: Rendered[] = g.nodes.map((n) => ({
    ...n,
    tier: tierOf.get(n.id) ?? 4,
    isFocus: n.id === data.term,
    entering: !prev.has(n.id),
    exiting: false,
  }))

  // diff against the *previous* layout (still in `prev`) before overwriting it,
  // so nodes that dropped out of the new neighbor set fade out from their last
  // known position instead of just vanishing
  const currentIDs = new Set(g.nodes.map((n) => n.id))
  const alreadyExiting = new Set(exiting.map((e) => e.id))
  const leavers: Rendered[] = []
  for (const [id, p] of prev) {
    if (!currentIDs.has(id) && !alreadyExiting.has(id)) {
      leavers.push({ id, r: 10, x: p.x, y: p.y, tier: 4, isFocus: false, entering: false, exiting: true })
    }
  }
  if (leavers.length > 0) pendingLeavers.current = leavers

  positions.current = new Map(g.nodes.map((n) => [n.id, { x: n.x, y: n.y }]))

  const edgeTargets = data.neighbors.map((n) => n.term)

  return (
    <section className="gv-panel">
      <div className="gv-subgraph">
        <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={`${data.term} 关联图`}>
          <g key={data.term} className="gv-edges">
            {g.edges.map((e, i) => (
              <line
                key={edgeTargets[i]}
                x1={e.x1} y1={e.y1} x2={e.x2} y2={e.y2}
                className={`gv-edge${hovered && hovered !== edgeTargets[i] ? ' dim' : ''}${hovered === edgeTargets[i] ? ' lit' : ''}`}
                strokeWidth={e.width}
              />
            ))}
          </g>
          {exiting.map((n) => (
            <g key={`exit-${n.id}`} className="gv-node exiting" transform={`translate(${n.x},${n.y})`}>
              <circle r={n.r} />
              <text y={n.r + 12} textAnchor="middle">{n.id}</text>
            </g>
          ))}
          {rendered.map((n) => (
            <g
              key={n.id}
              className={[
                'gv-node',
                `gv-ink-t${n.tier}`,
                n.isFocus ? 'focus' : '',
                n.entering ? 'enter' : '',
                hovered && hovered !== n.id && !n.isFocus ? 'dim' : '',
              ].filter(Boolean).join(' ')}
              transform={`translate(${n.x},${n.y})`}
              onClick={() => { if (!n.isFocus) onSelect(n.id) }}
              onMouseEnter={() => { if (!n.isFocus) setHovered(n.id) }}
              onMouseLeave={() => setHovered(null)}
            >
              <circle r={n.r} />
              <text y={n.r + 12} textAnchor="middle">{n.id}</text>
            </g>
          ))}
        </svg>
      </div>
      <div className="gv-items">
        <h3>「{data.term}」 · {data.count} 条相关</h3>
        {data.items.map((it) => (
          <div key={it.id} className="gv-item-row">
            <a href={it.permalink}>{it.title}</a>
            <span className="gv-item-meta">
              {it.source}
              {it.publishedAt ? ` · ${formatBeijingTime(it.publishedAt)}` : ''}
            </span>
          </div>
        ))}
      </div>
    </section>
  )
}
