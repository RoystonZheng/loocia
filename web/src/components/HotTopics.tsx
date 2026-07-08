import { useEffect, useState } from 'react'
import { fetchHotTopics, type HotTopic } from '../api/hot'

// HotTopics renders the 「当前热点」 strip; it disappears entirely when there
// are no topics or the fetch fails (the feed must not depend on it).
export function HotTopics() {
  const [topics, setTopics] = useState<HotTopic[]>([])

  useEffect(() => {
    fetchHotTopics()
      .then((res) => setTopics(res.items))
      .catch(() => setTopics([]))
  }, [])

  if (topics.length === 0) return null

  return (
    <section className="hot-topics">
      <h2 className="hot-title">⚡ 当前热点</h2>
      {topics.map((t, i) => (
        <div key={t.id} className="hot-row">
          <span className="hot-rank">{i + 1}</span>
          <a className="hot-link" href={t.permalink}>{t.title}</a>
          <span className="hot-badge">{t.sourceCount} 个信源</span>
        </div>
      ))}
    </section>
  )
}
