import { useEffect, useState } from 'react'
import { fetchVersion, type PublicVersion } from './api/version'
import { Feed } from './components/Feed'
import { DailyView } from './components/DailyView'
import { GraphView } from './components/GraphView'
import { Sidebar, type SidebarCounts, type View } from './components/Sidebar'
import { ToolsView, type ToolsSection } from './components/ToolsView'
import { fetchToolConfigs, fetchTools } from './api/tools'
import { useTheme } from './theme'

const HEADERS: Record<'selected' | 'all', { title: string; sub: string }> = {
  selected: { title: '精选', sub: 'AI 自动挑选的高价值内容' },
  all: { title: '全部 AI 动态', sub: 'AI 相关资讯全量信息流' },
}

function viewFromHash(): View {
  switch (window.location.hash) {
    case '#selected': return 'selected'
    case '#all': return 'all'
    case '#graph': return 'graph'
    case '#tools-configs': return 'tools-configs'
    case '#tools-discovered': return 'tools-discovered'
    case '#tools-evaluating': return 'tools-evaluating'
    case '#tools-team': return 'tools-team'
    default: return 'daily' // home
  }
}

function toolSectionFromView(view: View): ToolsSection | null {
  switch (view) {
    case 'tools-configs': return 'configs'
    case 'tools-discovered': return 'discovered'
    case 'tools-evaluating': return 'evaluating'
    case 'tools-team': return 'team'
    default: return null
  }
}

function viewFromToolSection(section: ToolsSection): View {
  return `tools-${section}` as View
}

function feedViewFromView(view: View): 'selected' | 'all' | null {
  return view === 'selected' || view === 'all' ? view : null
}

export default function App() {
  const [v, setV] = useState<PublicVersion | null>(null)
  const [view, setView] = useState<View>(viewFromHash)
  const [toolCounts, setToolCounts] = useState<SidebarCounts>({})
  useTheme()

  useEffect(() => {
    fetchVersion().then(setV).catch(() => {})
  }, [])

  useEffect(() => {
    let stale = false
    Promise.allSettled([
      fetchToolConfigs(),
      fetchTools({ status: 'discovered', take: 1 }),
      fetchTools({ status: 'evaluating', take: 1 }),
      fetchTools({ status: 'included', take: 1 }),
    ]).then(([configs, discovered, evaluating, team]) => {
      if (stale) return
      setToolCounts({
        configs: configs.status === 'fulfilled' ? configs.value.count : undefined,
        discovered: discovered.status === 'fulfilled' ? discovered.value.count : undefined,
        evaluating: evaluating.status === 'fulfilled' ? evaluating.value.count : undefined,
        team: team.status === 'fulfilled' ? team.value.count : undefined,
      })
    })
    return () => { stale = true }
  }, [])

  useEffect(() => {
    const sync = () => setView(viewFromHash())
    window.addEventListener('hashchange', sync)
    return () => window.removeEventListener('hashchange', sync)
  }, [])

  function handleView(v: View) {
    history.replaceState(null, '', v === 'daily' ? '/' : '#' + v)
    setView(v)
  }

  const toolSection = toolSectionFromView(view)
  const feedView = feedViewFromView(view)

  return (
    <div className="shell">
      <Sidebar view={view} onView={handleView} counts={toolCounts} />
      <div className={`main${toolSection ? ' tools-main' : ''}`}>
        {view === 'daily' ? (
          <DailyView />
        ) : view === 'graph' ? (
          <GraphView />
        ) : toolSection ? (
          <ToolsView section={toolSection} onSection={(next) => handleView(viewFromToolSection(next))} />
        ) : feedView ? (
          <>
            <header className="page-head">
              <h1>{HEADERS[feedView].title}</h1>
              <p className="page-sub">{HEADERS[feedView].sub}</p>
            </header>
            <Feed mode={feedView} />
          </>
        ) : (
          <DailyView />
        )}
        <footer className="app-footer">
          {v && <span>API v{v.apiVersion} · Skill v{v.skillVersion}</span>}
        </footer>
      </div>
    </div>
  )
}
