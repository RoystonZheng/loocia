import { useEffect, useState } from 'react'
import { fetchVersion, type PublicVersion } from './api/version'
import { Feed } from './components/Feed'
import { DailyView } from './components/DailyView'
import { GraphView } from './components/GraphView'
import { Sidebar, type View } from './components/Sidebar'
import { useTheme } from './theme'

const HEADERS: Record<Exclude<View, 'daily' | 'graph'>, { title: string; sub: string }> = {
  selected: { title: '精选', sub: 'AI 自动挑选的高价值内容' },
  all: { title: '全部 AI 动态', sub: 'AI 相关资讯全量信息流' },
}

function viewFromHash(): View {
  switch (window.location.hash) {
    case '#all': return 'all'
    case '#daily': return 'daily'
    case '#graph': return 'graph'
    default: return 'selected'
  }
}

export default function App() {
  const [v, setV] = useState<PublicVersion | null>(null)
  const [view, setView] = useState<View>(viewFromHash)
  const [theme, setTheme] = useTheme()

  useEffect(() => {
    fetchVersion().then(setV).catch(() => {})
  }, [])

  function handleView(v: View) {
    history.replaceState(null, '', v === 'selected' ? '/' : '#' + v)
    setView(v)
  }

  return (
    <div className="shell">
      <Sidebar view={view} onView={handleView} theme={theme} onTheme={setTheme} />
      <div className="main">
        {view === 'daily' ? (
          <DailyView />
        ) : view === 'graph' ? (
          <GraphView />
        ) : (
          <>
            <header className="page-head">
              <h1>{HEADERS[view].title}</h1>
              <p className="page-sub">{HEADERS[view].sub}</p>
            </header>
            <Feed mode={view} />
          </>
        )}
        <footer className="app-footer">
          {v && <span>API v{v.apiVersion} · Skill v{v.skillVersion}</span>}
        </footer>
      </div>
    </div>
  )
}
