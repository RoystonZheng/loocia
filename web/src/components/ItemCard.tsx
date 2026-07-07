import type { PublicItem } from '../api/items'
import { categoryLabel } from '../format'

export function ItemCard({ item }: { item: PublicItem }) {
  return (
    <article className="card">
      <div className="card-top">
        <span className="card-source">
          <span className="card-avatar" aria-hidden />
          {item.source}
        </span>
        <div className="card-badges">
          {item.selected && <span className="badge-sel">✦ 精选</span>}
          {item.score != null && <span className="badge-score">{item.score}</span>}
        </div>
      </div>

      <a className="card-title" href={item.permalink}>{item.title}</a>

      {item.summary && <p className="card-summary">{item.summary}</p>}

      {item.imageUrl && (
        <img
          className="card-image"
          src={item.imageUrl}
          alt=""
          loading="lazy"
          // External CDN images can 404/hotlink-block; hide rather than show a broken icon.
          onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = 'none' }}
        />
      )}

      {item.category && (
        <div className="card-tags">
          <span className="tag">{categoryLabel(item.category)}</span>
        </div>
      )}
    </article>
  )
}
