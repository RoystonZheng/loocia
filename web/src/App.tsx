import { useEffect, useState } from 'react'
import { fetchVersion, type PublicVersion } from './api/version'
import { Feed } from './components/Feed'
import { DailyView } from './components/DailyView'
import { HotTopics } from './components/HotTopics'

type View = 'feed' | 'daily'

export default function App() {
  const [v, setV] = useState<PublicVersion | null>(null)
  const [view, setView] = useState<View>('feed')
  useEffect(() => {
    fetchVersion().then(setV).catch(() => {})
  }, [])
  return (
    <div className="app">
      <header className="app-header">
        <h1>AI HOT（内网）</h1>
        <p className="app-sub">AI 资讯精选</p>
        <nav className="app-nav">
          <button className={view === 'feed' ? 'active' : ''} onClick={() => setView('feed')}>资讯流</button>
          <button className={view === 'daily' ? 'active' : ''} onClick={() => setView('daily')}>日报</button>
        </nav>
      </header>
      <main>{view === 'feed' ? (<><HotTopics /><Feed /></>) : <DailyView />}</main>
      <footer className="app-footer">
        {v && <span>API v{v.apiVersion} · Skill v{v.skillVersion}</span>}
      </footer>
    </div>
  )
}
