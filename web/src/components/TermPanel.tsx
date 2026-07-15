import { useEffect, useMemo, useRef, useState } from 'react'
import { flushSync } from 'react-dom'
import { fetchTerm, type GraphWindow, type TermResponse } from '../api/graph'
import { layoutGraph, type GraphEdge, type GraphNode, type PlacedNode } from '../graphLayout'
import { inkTier } from '../ink'
import { formatBeijingTime } from '../format'

const W = 420
const H = 320
const FOCUS_R = 26
const EXIT_MS = 400

type Tier = 1 | 2 | 3 | 4

interface Rendered extends PlacedNode {
  tier: Tier
  isFocus: boolean
  entering: boolean
}

// last committed geometry per node, kept across data changes so the next
// layout can seed from it and leavers can fade out from where they stood
interface NodeSnapshot {
  x: number
  y: number
  r: number
  tier: Tier
}

interface ExitNode extends NodeSnapshot {
  id: string
  // exit nodes mount un-armed (no .exiting class, full opacity) and get the
  // class one commit later so the CSS opacity transition actually plays
  armed: boolean
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
  const [exiting, setExiting] = useState<ExitNode[]>([])
  // snapshot of the last committed layout; ONLY written in the effect keyed on
  // `layout`, so the memo below reads a stable value between data changes
  const positions = useRef(new Map<string, NodeSnapshot>())
  const svgRef = useRef<SVGSVGElement>(null)

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

  // geometry is frozen between data changes: hover-only re-renders must NOT
  // re-run the force simulation (it reheats and drifts nodes a few px per run)
  const layout = useMemo(() => {
    if (!data || data.count === 0) return null
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

    const tierOf = new Map<string, Tier>([
      [data.term, 1],
      ...data.neighbors.map((n) => [n.term, inkTier(n.weight, maxW)] as const),
    ])
    const rendered: Rendered[] = g.nodes.map((n) => ({
      ...n,
      tier: tierOf.get(n.id) ?? 4,
      isFocus: n.id === data.term,
      entering: !prev.has(n.id),
    }))

    // diff against the previous snapshot: nodes that dropped out of the new
    // neighbor set fade out from their last known position, in their own shade
    const next = new Map(rendered.map((n) => [n.id, { x: n.x, y: n.y, r: n.r, tier: n.tier }]))
    const leavers: ExitNode[] = []
    for (const [id, p] of prev) {
      if (!next.has(id)) leavers.push({ id, ...p, armed: false })
    }
    return { edges: g.edges, rendered, next, leavers, edgeTargets: data.neighbors.map((n) => n.term) }
  }, [data])

  // commit the new snapshot + stage leavers exactly once per data change
  useEffect(() => {
    if (!layout) return
    positions.current = layout.next
    setExiting((old) => {
      // a node re-entering the graph must stop exiting; never double-stage
      const kept = old.filter((e) => !layout.next.has(e.id))
      const have = new Set(kept.map((e) => e.id))
      const add = layout.leavers.filter((l) => !have.has(l.id))
      return add.length > 0 || kept.length !== old.length ? [...kept, ...add] : old
    })
  }, [layout])

  // arm freshly-mounted exit nodes: force a style recalc between the un-armed
  // mount and the class flip so the opacity transition fires (styles already
  // present at mount never transition)
  useEffect(() => {
    if (!exiting.some((e) => !e.armed)) return
    void svgRef.current?.getBoundingClientRect()
    setExiting((old) => old.map((e) => (e.armed ? e : { ...e, armed: true })))
  }, [exiting])

  // exit nodes leave the DOM after their fade-out completes
  useEffect(() => {
    if (exiting.length === 0 || exiting.some((e) => !e.armed)) return
    const t = setTimeout(() => { flushSync(() => setExiting([])) }, EXIT_MS)
    return () => clearTimeout(t)
  }, [exiting])

  if (failed) return <div className="gv-panel gv-status">加载失败，稍后再试</div>
  if (!data) return <div className="gv-panel gv-status">{loading ? '加载中…' : '该时间段暂无数据'}</div>
  if (!layout) return <div className="gv-panel gv-status">该时间段暂无数据</div>

  return (
    <section className="gv-panel">
      <div className="gv-subgraph">
        <svg ref={svgRef} viewBox={`0 0 ${W} ${H}`} role="img" aria-label={`${data.term} 关联图`}>
          <g key={data.term} className="gv-edges">
            {layout.edges.map((e, i) => (
              <line
                key={layout.edgeTargets[i]}
                x1={e.x1} y1={e.y1} x2={e.x2} y2={e.y2}
                className={`gv-edge${hovered && hovered !== layout.edgeTargets[i] ? ' dim' : ''}${hovered === layout.edgeTargets[i] ? ' lit' : ''}`}
                strokeWidth={e.width}
              />
            ))}
          </g>
          {exiting.map((n) => (
            <g
              key={`exit-${n.id}`}
              className={`gv-node gv-ink-t${n.tier}${n.armed ? ' exiting' : ''}`}
              transform={`translate(${n.x},${n.y})`}
            >
              <circle r={n.r} />
              <text y={n.r + 12} textAnchor="middle">{n.id}</text>
            </g>
          ))}
          {layout.rendered.map((n, i) => (
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
              {/* 内层承载漂浮动画，外层 transform 专管定位过渡，二者不抢 transform */}
              <g
                className="gv-node-inner"
                style={{ '--fd': `${5 + (i % 5) * 0.7}s`, '--fdelay': `${(i % 7) * 0.45}s` } as React.CSSProperties}
              >
                <circle r={n.r} />
                <text y={n.r + 12} textAnchor="middle">{n.id}</text>
              </g>
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
