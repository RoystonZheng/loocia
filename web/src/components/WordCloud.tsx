import { useEffect, useState } from 'react'
import { fetchCloud, type CloudTerm, type GraphWindow } from '../api/graph'
import { layoutCloud, type PlacedWord } from '../cloudLayout'
import { inkTier } from '../ink'
import { TermPanel } from './TermPanel'

const CLOUD_W = 900
const DEFAULT_H = 420
const MIN_FONT = 14
const MAX_FONT = 48

const WINDOWS: { key: GraphWindow; label: string }[] = [
  { key: '7d', label: '7 天' },
  { key: '30d', label: '30 天' },
  { key: 'all', label: '全部' },
]

// fontSize maps count→px on a sqrt scale so the head doesn't drown the tail.
function fontSize(count: number, maxCount: number): number {
  return MIN_FONT + (MAX_FONT - MIN_FONT) * Math.sqrt(count / maxCount)
}

// WordCloud renders the interactive keyword cloud (size = frequency in the
// window) with a time-range selector; clicking a word opens its co-occurrence
// panel. Reused by the 图谱 page and the home hero (pass a shorter `height`).
export function WordCloud({ height = DEFAULT_H }: { height?: number }) {
  const [win, setWin] = useState<GraphWindow>('7d')
  const [words, setWords] = useState<PlacedWord[] | null>(null)
  const [empty, setEmpty] = useState(false)
  const [failed, setFailed] = useState(false)
  const [selected, setSelected] = useState<string | null>(null)

  useEffect(() => {
    let stale = false
    setWords(null)
    setEmpty(false)
    setFailed(false)
    fetchCloud(win)
      .then(async (res) => {
        if (stale) return
        if (res.terms.length === 0) {
          setEmpty(true)
          return
        }
        const maxCount = res.terms[0].count
        const placed = await layoutCloud(
          res.terms.map((t: CloudTerm) => ({
            text: t.term,
            size: fontSize(t.count, maxCount),
            kind: t.kind,
            tier: inkTier(t.count, maxCount),
          })),
          CLOUD_W, height,
        )
        if (!stale) setWords(placed)
      })
      .catch(() => { if (!stale) setFailed(true) })
    return () => { stale = true }
  }, [win, height])

  return (
    <>
      <div className="gv-controls" role="group" aria-label="时间范围">
        {WINDOWS.map((w) => (
          <button
            key={w.key}
            className={`gv-win-btn${win === w.key ? ' active' : ''}`}
            aria-pressed={win === w.key}
            onClick={() => setWin(w.key)}
          >
            {w.label}
          </button>
        ))}
      </div>

      {failed && <div className="gv-status">加载失败，稍后再试</div>}
      {empty && <div className="gv-status">该时间段暂无数据</div>}
      {!failed && !empty && !words && <div className="gv-status">加载中…</div>}

      {words && (
        <svg
          className="gv-cloud"
          viewBox={`${-CLOUD_W / 2} ${-height / 2} ${CLOUD_W} ${height}`}
          role="img"
          aria-label="关键词云图"
        >
          {words.map((w, i) => (
            <text
              key={w.text}
              x={w.x}
              y={w.y}
              fontSize={w.size}
              textAnchor="middle"
              className={`gv-word gv-ink-t${w.tier}${selected === w.text ? ' selected' : ''}`}
              // 呼吸漂浮：周期/相位按序号错开，词与词不同步（确定性，无随机）
              style={{ '--fd': `${5 + (i % 5) * 0.7}s`, '--fdelay': `${(i % 7) * 0.45}s` } as React.CSSProperties}
              onClick={() => setSelected(w.text)}
            >
              {w.text}
            </text>
          ))}
        </svg>
      )}

      {selected && <TermPanel term={selected} window={win} onSelect={setSelected} />}
    </>
  )
}
