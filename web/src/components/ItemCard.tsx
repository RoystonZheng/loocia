import type { PublicItem } from '../api/items'
import { categoryLabel, scoreLabel, scoreTier } from '../format'

export function ItemCard({ item }: { item: PublicItem }) {
  return (
    <article className="card">
      <div className="card-top">
        <span className="card-source">
          <span className="card-avatar" aria-hidden />
          {item.source}
        </span>
        <div className="card-badges">
          {item.sourceKind === 'aihot' && <span className="badge-source-kind">AIHOT补漏</span>}
          {item.selected && <span className="badge-sel">✦ 精选</span>}
          {/* The score is a "why this is 精选" quality signal — only meaningful on
              curated items. Showing it on every 全部 item just advertises low
              numbers (e.g. an 18 on a barely-AI story), so gate it on selected. */}
          {item.selected && item.score != null && (
            <span className="badge-score" data-tier={scoreTier(item.score)}>{scoreLabel(item.score)}</span>
          )}
        </div>
      </div>

      <a className="card-title" href={item.permalink}>{item.title}</a>

      {item.summary && <p className="card-summary">{item.summary}</p>}

      {item.imageUrl ? (
        <a className="card-media" href={item.permalink} aria-label={item.videoUrl ? '播放视频' : undefined}>
          <img
            className="card-image"
            src={item.imageUrl}
            alt=""
            loading="lazy"
            // WeChat (mmbiz.qpic.cn) serves an anti-hotlink placeholder unless the
            // request carries no Referer; no-referrer gets the real cover image.
            referrerPolicy="no-referrer"
            // External CDN images can 404/hotlink-block; hide rather than show a broken icon.
            onError={(e) => {
              const wrap = (e.currentTarget as HTMLImageElement).closest('.card-media') as HTMLElement | null
              if (wrap) wrap.style.display = 'none'
            }}
          />
          {item.videoUrl && <span className="card-play" aria-hidden>▶</span>}
        </a>
      ) : (
        item.videoUrl && (
          <a className="card-media card-media-bare" href={item.permalink} aria-label="播放视频">
            <span className="card-play" aria-hidden>▶</span>
            <span className="card-video-tag">视频</span>
          </a>
        )
      )}

      {item.category && (
        <div className="card-tags">
          <span className="tag">{categoryLabel(item.category)}</span>
        </div>
      )}
    </article>
  )
}
