import type { PublicItem } from '../api/items'
import { formatBeijingTime, categoryLabel } from '../format'

export function ItemCard({ item }: { item: PublicItem }) {
  return (
    <article className="item-card">
      <div className="item-head">
        <a className="item-title" href={item.permalink}>{item.title}</a>
        {item.score != null && <span className="item-score">{item.score}</span>}
      </div>
      <div className="item-meta">
        <span className="item-source">{item.source}</span>
        {item.category && <span className="item-cat">{categoryLabel(item.category)}</span>}
        {item.publishedAt && <span className="item-time">{formatBeijingTime(item.publishedAt)}</span>}
      </div>
      {item.summary && <p className="item-summary">{item.summary}</p>}
      {item.imageUrl && (
        <img
          className="item-image"
          src={item.imageUrl}
          alt=""
          loading="lazy"
          // External CDN images can 404/hotlink-block; hide rather than show a broken icon.
          onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = 'none' }}
        />
      )}
    </article>
  )
}
