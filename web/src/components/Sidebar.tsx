import type { Theme } from '../theme'

export type View = 'selected' | 'all' | 'daily'

const NAV: { view: View; label: string; icon: string }[] = [
  { view: 'selected', label: '精选', icon: '✦' },
  { view: 'all', label: '全部 AI 动态', icon: '≣' },
  { view: 'daily', label: 'AI 日报', icon: '▤' },
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
        <span className="sb-logo-dot">◉</span>
        <span className="sb-logo-hot">HOT</span>
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
