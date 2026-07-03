import { useEffect, useState } from 'react'
import { fetchVersion, type PublicVersion } from './api/version'

export default function App() {
  const [v, setV] = useState<PublicVersion | null>(null)
  useEffect(() => { fetchVersion().then(setV).catch(() => {}) }, [])
  return (
    <main>
      <h1>AI HOT（内网）</h1>
      {v && <p>API v{v.apiVersion} · Skill v{v.skillVersion}</p>}
    </main>
  )
}
