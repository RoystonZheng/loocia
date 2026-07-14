import { useEffect, useState } from 'react'
import { fetchTerm, type GraphWindow, type TermResponse } from '../api/graph'
import { layoutGraph, type GraphEdge, type GraphNode } from '../graphLayout'
import { formatBeijingTime } from '../format'

const W = 420
const H = 320
const FOCUS_R = 26

// TermPanel is the drill-down under the cloud: a static force-directed
// co-occurrence subgraph (click a neighbor to refocus) + the term's newest items.
export function TermPanel({
  term, window: win, onSelect,
}: {
  term: string
  window: GraphWindow
  onSelect: (term: string) => void
}) {
  const [data, setData] = useState<TermResponse | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let stale = false
    setData(null)
    setFailed(false)
    fetchTerm(term, win)
      .then((d) => { if (!stale) setData(d) })
      .catch(() => { if (!stale) setFailed(true) })
    return () => { stale = true }
  }, [term, win])

  if (failed) return <div className="gv-panel gv-status">加载失败，稍后再试</div>
  if (!data) return <div className="gv-panel gv-status">加载中…</div>
  if (data.count === 0) return <div className="gv-panel gv-status">该时间段暂无数据</div>

  const maxW = Math.max(...data.neighbors.map((n) => n.weight), 1)
  const nodes: GraphNode[] = [
    { id: data.term, r: FOCUS_R },
    ...data.neighbors.map((n) => ({ id: n.term, r: 10 + 12 * (n.weight / maxW) })),
  ]
  const edges: GraphEdge[] = data.neighbors.map((n) => ({
    source: data.term, target: n.term, width: 1 + 3 * (n.weight / maxW),
  }))
  const g = layoutGraph(nodes, edges, W, H)
  const kindOf = new Map(data.neighbors.map((n) => [n.term, n.kind]))

  return (
    <section className="gv-panel">
      <div className="gv-subgraph">
        <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={`${data.term} 关联图`}>
          {g.edges.map((e, i) => (
            <line key={i} x1={e.x1} y1={e.y1} x2={e.x2} y2={e.y2} className="gv-edge" strokeWidth={e.width} />
          ))}
          {g.nodes.map((n) => {
            const isFocus = n.id === data.term
            const kind = isFocus ? data.kind : kindOf.get(n.id)
            return (
              <g
                key={n.id}
                className={`gv-node${isFocus ? ' focus' : ''} ${kind === 'entity' ? 'entity' : 'topic'}`}
                onClick={() => { if (!isFocus) onSelect(n.id) }}
              >
                <circle cx={n.x} cy={n.y} r={n.r} />
                <text x={n.x} y={n.y + n.r + 12} textAnchor="middle">{n.id}</text>
              </g>
            )
          })}
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
