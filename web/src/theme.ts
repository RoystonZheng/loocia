import { useEffect, useState } from 'react'

export type Theme = 'light' | 'dark' | 'system'

const KEY = 'aihot-theme'

function systemDark(): boolean {
  return typeof matchMedia !== 'undefined' && matchMedia('(prefers-color-scheme: dark)').matches
}

function readTheme(): Theme {
  try {
    return (localStorage.getItem(KEY) as Theme) || 'system'
  } catch {
    return 'system'
  }
}

// apply stamps data-theme on <html> so CSS variables resolve; 'system' follows the OS.
function apply(theme: Theme) {
  if (typeof document === 'undefined') return
  const effective = theme === 'system' ? (systemDark() ? 'dark' : 'light') : theme
  document.documentElement.dataset.theme = effective
}

// useTheme persists a 3-state theme and keeps <html data-theme> in sync, including
// live OS changes while on 'system'.
export function useTheme(): [Theme, (t: Theme) => void] {
  const [theme, setTheme] = useState<Theme>(readTheme)

  useEffect(() => {
    apply(theme)
    try { localStorage.setItem(KEY, theme) } catch { /* ignore */ }
    if (theme !== 'system' || typeof matchMedia === 'undefined') return
    const mq = matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => apply('system')
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [theme])

  return [theme, setTheme]
}
