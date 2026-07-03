import { useCallback, useEffect, useState } from 'react'
import { fetchItems, type PublicItem } from '../api/items'
import { ItemCard } from './ItemCard'
import { categoryLabel } from '../format'

const CATEGORIES = ['ai-models', 'ai-products', 'industry', 'paper', 'tip']
const PAGE_SIZE = 20

export function Feed() {
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
    [submittedQ, category],
  )

  useEffect(() => {
    load(true, null)
  }, [load])

  return (
    <div className="feed">
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
          placeholder="搜索 AI 资讯…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <button type="submit">搜索</button>
      </form>

      <div className="feed-cats">
        <button
          className={category === undefined ? 'active' : ''}
          onClick={() => setCategory(undefined)}
        >
          全部
        </button>
        {CATEGORIES.map((c) => (
          <button
            key={c}
            className={category === c ? 'active' : ''}
            onClick={() => setCategory(c)}
          >
            {categoryLabel(c)}
          </button>
        ))}
      </div>

      {error && <p className="feed-error">加载失败，请稍后重试。</p>}

      <div className="feed-list">
        {items.map((it) => (
          <ItemCard key={it.id} item={it} />
        ))}
      </div>

      {!error && items.length === 0 && !loading && <p className="feed-empty">暂无资讯。</p>}

      {hasNext && (
        <button
          className="feed-more"
          disabled={loading}
          onClick={() => load(false, cursor)}
        >
          {loading ? '加载中…' : '加载更多'}
        </button>
      )}
    </div>
  )
}
