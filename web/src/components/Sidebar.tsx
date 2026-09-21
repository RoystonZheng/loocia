export type View = 'selected' | 'all' | 'daily' | 'graph'
  | 'tools-configs' | 'tools-accounts' | 'tools-discovered' | 'tools-evaluating' | 'tools-team'

export interface SidebarCounts {
  configs?: number
  discovered?: number
  team?: number
}

type BadgeKey = keyof SidebarCounts

const NAV_GROUPS: {
  title: string
  items: { view: View; label: string; icon: string; badge?: BadgeKey }[]
}[] = [
  {
    title: '内容',
    items: [
      { view: 'daily', label: 'AI 日报', icon: '▤' },
      { view: 'all', label: 'AI 动态', icon: '≣' },
      { view: 'graph', label: '话题图谱', icon: '◇' },
    ],
  },
  {
    title: '工具',
    items: [
      { view: 'tools-discovered', label: '工具百宝箱', icon: '◎', badge: 'discovered' },
      { view: 'tools-team', label: '团队工具', icon: '⌁', badge: 'team' },
    ],
  },
  {
    title: '设置',
    items: [
      { view: 'tools-configs', label: '发现配置', icon: '☷', badge: 'configs' },
    ],
  },
]

export function Sidebar({
  view, onView, counts = {},
}: {
  view: View
  onView: (v: View) => void
  counts?: SidebarCounts
}) {
  return (
    <aside className="sidebar">
      <div className="sb-logo">
        <span className="sb-logo-ai">AI</span>
        <svg className="sb-logo-mark" width="18" height="18" viewBox="0 0 20 20" aria-hidden="true">
          <defs>
            <linearGradient id="sbLogoMark" x1="0" y1="0" x2="1" y2="1">
              <stop offset="0" stopColor="#5ab6ff" />
              <stop offset="1" stopColor="#2f6bff" />
            </linearGradient>
          </defs>
          <path d="M10 1.5C10 6 10 6 14.2 8 10 10 10 10 10 18.5 10 10 10 10 5.8 8 10 6 10 6 10 1.5Z" fill="url(#sbLogoMark)" />
        </svg>
        <span className="sb-logo-hot">Cool</span>
        <small>2.0</small>
      </div>

      <nav className="sb-nav" aria-label="主导航">
        {NAV_GROUPS.map((group) => (
          <div key={group.title} className="sb-nav-group">
            <div className="sb-group">{group.title}</div>
            {group.items.map((item) => {
              const badge = item.badge ? counts[item.badge] : undefined
              const active = view === item.view || (item.view === 'all' && view === 'selected')
              return (
                <button
                  key={item.view}
                  className={`sb-item${active ? ' active' : ''}`}
                  onClick={() => onView(item.view)}
                >
                  <span className="sb-icon" aria-hidden>{item.icon}</span>
                  <span>{item.label}</span>
                  {badge != null && <span className="sb-badge" aria-hidden>{badge}</span>}
                </button>
              )
            })}
          </div>
        ))}
      </nav>

      <div className="sb-foot">
        <strong>AI 小组</strong>
        <span>内部最佳实践</span>
      </div>
    </aside>
  )
}
