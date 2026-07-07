import type { ReactNode } from 'react'
import { useCallback, useEffect, useState } from 'react'
import { fetchItems, type PublicItem } from '../api/items'
import { ItemCard } from './ItemCard'
import { HotTopics } from './HotTopics'
import { categoryLabel, beijingParts } from '../format'

const CATEGORIES = ['ai-models', 'ai-products', 'industry', 'paper', 'tip']
const PAGE_SIZE = 20

export function Feed({ mode }: { mode: 'selected' | 'all' }) {
  const [items, setItems] = useState<PublicItem[]>([])
  const [cursor, setCursor] = useState<string | null>(null)
  const [hasNext, setHasNext] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(false)
  const [q, setQ] = useState('')
  const [submittedQ, setSubmittedQ] = useState('')
  const [category, setCategory] = useState<string | undefined>(undefined)

  const load = useCallback(
    async (reset: boolean, curCursor: string | null) => {
      setLoading(true)
      setError(false)
      try {
        const res = await fetchItems({
          mode,
          take: PAGE_SIZE,
          q: submittedQ || undefined,
          category,
          cursor: reset ? undefined : curCursor ?? undefined,
        })
        setItems((prev) => (reset ? res.items : [...prev, ...res.items]))
        setCursor(res.nextCursor)
        setHasNext(res.hasNext)
      } catch {
        setError(true)
      } finally {
        setLoading(false)
      }
    },
    [mode, submittedQ, category],
  )

  useEffect(() => {
    load(true, null)
  }, [load])

  return (
    <div className="feed">
      {mode === 'selected' && <HotTopics />}

      <div className="feed-controls">
        <div className="feed-cats">
          <button
            className={category === undefined ? 'chip active' : 'chip'}
            onClick={() => setCategory(undefined)}
          >
            全部
          </button>
          {CATEGORIES.map((c) => (
            <button
              key={c}
              className={category === c ? 'chip active' : 'chip'}
              onClick={() => setCategory(c)}
            >
              {categoryLabel(c)}
            </button>
          ))}
        </div>
        <form
          role="search"
          className="feed-search"
          onSubmit={(e) => {
            e.preventDefault()
            setSubmittedQ(q.trim())
          }}
        >
          <input
            type="search"
            placeholder="搜索标题/摘要/正文…"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
          <button type="submit">搜索</button>
        </form>
      </div>

      {error && <p className="feed-error">加载失败，请稍后重试。</p>}

      <div className="timeline">{renderTimeline(items)}</div>

      {!error && items.length === 0 && !loading && <p className="feed-empty">暂无资讯。</p>}

      {hasNext && (
        <button className="feed-more" disabled={loading} onClick={() => load(false, cursor)}>
          {loading ? '加载中…' : '加载更多'}
        </button>
      )}
    </div>
  )
}

// renderTimeline lays items on a time rail, inserting a date divider whenever the
// Beijing calendar day changes.
function renderTimeline(items: PublicItem[]): ReactNode[] {
  const out: ReactNode[] = []
  let lastDate = ''
  for (const it of items) {
    const { date, clock, monthDay } = beijingParts(it.publishedAt)
    if (date && date !== lastDate) {
      lastDate = date
      out.push(
        <div key={`d-${date}`} className="tl-divider">
          {monthDay}
        </div>,
      )
    }
    out.push(
      <div key={it.id} className="tl-row">
        <div className="tl-time">{clock}</div>
        <div className="tl-rail">
          <span className="tl-dot" />
        </div>
        <div className="tl-card-wrap">
          <ItemCard item={it} />
        </div>
      </div>,
    )
  }
  return out
}
