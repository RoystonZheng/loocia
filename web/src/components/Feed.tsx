import type { ReactNode } from 'react'
import { useCallback, useEffect, useState } from 'react'
import { fetchItems, type PublicItem } from '../api/items'
import { ItemCard } from './ItemCard'
import { HotTopics } from './HotTopics'
import { categoryLabel, beijingParts } from '../format'

const CATEGORIES = ['ai-models', 'ai-products', 'industry', 'paper', 'tip']
type SourceKind = 'rss' | 'html' | 'mp' | 'aihot'

const SOURCES: { key: SourceKind | undefined; label: string }[] = [
  { key: undefined, label: '全部来源' },
  { key: 'rss', label: 'RSS/Atom' },
  { key: 'html', label: '网页直采' },
  { key: 'mp', label: '公众号' },
  { key: 'aihot', label: 'AIHOT补漏' },
]
const PAGE_SIZE = 20
type ArticleFilter = 'all' | 'selected' | 'score-3' | 'score-4' | 'score-5'

const ARTICLE_FILTERS: { value: ArticleFilter; label: string }[] = [
  { value: 'all', label: '全部文章' },
  { value: 'selected', label: '精选' },
  { value: 'score-3', label: '评分 3 分以上' },
  { value: 'score-4', label: '评分 4 分以上' },
  { value: 'score-5', label: '评分 5 分以上' },
]

function filterQuery(filter: ArticleFilter): { mode: 'selected' | 'all'; scoreMin?: number } {
  if (filter === 'selected') return { mode: 'selected' }
  if (filter.startsWith('score-')) return { mode: 'all', scoreMin: Number(filter.slice('score-'.length)) }
  return { mode: 'all' }
}

export function Feed({ mode }: { mode: 'selected' | 'all' }) {
  const [items, setItems] = useState<PublicItem[]>([])
  const [cursor, setCursor] = useState<string | null>(null)
  const [hasNext, setHasNext] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(false)
  const [q, setQ] = useState('')
  const [submittedQ, setSubmittedQ] = useState('')
  const [category, setCategory] = useState<string | undefined>(undefined)
  const [sourceKind, setSourceKind] = useState<SourceKind | undefined>(undefined)
  const [articleFilter, setArticleFilter] = useState<ArticleFilter>(mode === 'selected' ? 'selected' : 'all')

  const load = useCallback(
    async (reset: boolean, curCursor: string | null) => {
      setLoading(true)
      setError(false)
      try {
        const filter = filterQuery(articleFilter)
        const res = await fetchItems({
          ...filter,
          take: PAGE_SIZE,
          q: submittedQ || undefined,
          category,
          sourceKind,
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
    [articleFilter, submittedQ, category, sourceKind],
  )

  useEffect(() => {
    load(true, null)
  }, [load])

  return (
    <div className="feed">
      <HotTopics />

      <div className="feed-controls">
        <div className="feed-sources">
          {SOURCES.map((s) => (
            <button
              key={s.label}
              className={sourceKind === s.key ? 'chip active' : 'chip'}
              onClick={() => setSourceKind(s.key)}
            >
              {s.label}
            </button>
          ))}
        </div>
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
        <div className="feed-query-actions">
          <label className="feed-filter">
            <span>筛选</span>
            <select
              aria-label="资讯筛选"
              value={articleFilter}
              onChange={(e) => setArticleFilter(e.target.value as ArticleFilter)}
            >
              {ARTICLE_FILTERS.map((filter) => (
                <option key={filter.value} value={filter.value}>{filter.label}</option>
              ))}
            </select>
          </label>
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
