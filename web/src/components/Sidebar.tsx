import type { Theme } from '../theme'

export type View = 'selected' | 'all' | 'daily' | 'graph'

const NAV: { view: View; label: string; icon: string }[] = [
  { view: 'daily', label: 'AI 日报', icon: '▤' },
  { view: 'selected', label: '精选', icon: '✦' },
  { view: 'all', label: '全部 AI 动态', icon: '≣' },
  { view: 'graph', label: '图谱', icon: '❖' },
]

const THEMES: { key: Theme; glyph: string; title: string }[] = [
  { key: 'dark', glyph: '☾', title: '深色' },
  { key: 'system', glyph: '▢', title: '跟随系统' },
  { key: 'light', glyph: '☀', title: '浅色' },
]

export function Sidebar({
  view, onView, theme, onTheme,
}: {
  view: View
  onView: (v: View) => void
  theme: Theme
  onTheme: (t: Theme) => void
}) {
  return (
    <aside className="sidebar">
      <div className="sb-logo">
        <span className="sb-logo-ai">AI</span>
        <svg className="sb-logo-mark" width="21" height="21" viewBox="0 0 20 20" aria-hidden="true">
          <defs>
            <linearGradient id="sbLogoMark" x1="0" y1="0" x2="1" y2="1">
              <stop offset="0" stopColor="#5ab6ff" />
              <stop offset="1" stopColor="#2f6bff" />
            </linearGradient>
          </defs>
          <path d="M10 1.5C10 6 10 6 14.2 8 10 10 10 10 10 18.5 10 10 10 10 5.8 8 10 6 10 6 10 1.5Z" fill="url(#sbLogoMark)" />
        </svg>
        <span className="sb-logo-hot">Cool</span>
      </div>

      <nav className="sb-nav">
        <div className="sb-group">内容</div>
        {NAV.map((n) => (
          <button
            key={n.view}
            className={`sb-item${view === n.view ? ' active' : ''}`}
            onClick={() => onView(n.view)}
          >
            <span className="sb-icon" aria-hidden>{n.icon}</span>
            {n.label}
          </button>
        ))}
      </nav>

      <div className="sb-theme" role="group" aria-label="主题">
        {THEMES.map((t) => (
          <button
            key={t.key}
            className={`sb-theme-btn${theme === t.key ? ' active' : ''}`}
            title={t.title}
            aria-label={t.title}
            aria-pressed={theme === t.key}
            onClick={() => onTheme(t.key)}
          >
            {t.glyph}
          </button>
        ))}
      </div>
    </aside>
  )
}
