import { useEffect, useState } from 'react'
import { fetchVersion, type PublicVersion } from './api/version'
import { Feed } from './components/Feed'

export default function App() {
  const [v, setV] = useState<PublicVersion | null>(null)
  useEffect(() => {
    fetchVersion().then(setV).catch(() => {})
  }, [])
  return (
    <div className="app">
      <header className="app-header">
        <h1>AI HOT（内网）</h1>
        <p className="app-sub">AI 资讯精选</p>
      </header>
      <main>
        <Feed />
      </main>
      <footer className="app-footer">
        {v && <span>API v{v.apiVersion} · Skill v{v.skillVersion}</span>}
      </footer>
    </div>
  )
}
