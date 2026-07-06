import { useEffect, useState } from 'react'
import { fetchDailyList, fetchDailyByDate, type DailyReport } from '../api/daily'
import { formatBeijingTime } from '../format'

type State = 'loading' | 'ready' | 'empty' | 'error'

export function DailyView() {
  const [state, setState] = useState<State>('loading')
  const [dates, setDates] = useState<string[]>([]) // newest first
  const [idx, setIdx] = useState(0)
  const [report, setReport] = useState<DailyReport | null>(null)

  // Load the archive index once; empty index → empty state.
  useEffect(() => {
    fetchDailyList()
      .then((list) => {
        if (list.length === 0) setState('empty')
        else setDates(list.map((d) => d.date))
      })
      .catch(() => setState('error'))
  }, [])

  // Load the selected day's full report whenever the index changes.
  useEffect(() => {
    if (dates.length === 0) return
    setState('loading')
    fetchDailyByDate(dates[idx])
      .then((rep) => {
        if (rep === null) {
          setState('empty')
        } else {
          setReport(rep)
          setState('ready')
        }
      })
      .catch(() => setState('error'))
  }, [dates, idx])

  if (state === 'error') return <p className="daily-status feed-error">加载失败，请稍后重试。</p>
  if (state === 'empty') return <p className="daily-status">暂无日报。</p>
  if (dates.length === 0) return <p className="daily-status">加载中…</p>

  // dates known: the archive nav is always shown; the body swaps per selection.
  const nav = (
    <div className="daily-nav">
      <button className="daily-nav-btn" disabled={idx >= dates.length - 1} onClick={() => setIdx((i) => i + 1)}>
        ← 前一天
      </button>
      <span className="daily-nav-date">
        {dates[idx]}
        {dates.length > 1 ? ` · ${idx + 1}/${dates.length}` : ''}
      </span>
      <button className="daily-nav-btn" disabled={idx <= 0} onClick={() => setIdx((i) => i - 1)}>
        后一天 →
      </button>
    </div>
  )

  if (state !== 'ready' || report === null) {
    return (
      <div className="daily">
        {nav}
        <p className="daily-status">加载中…</p>
      </div>
    )
  }

  const rep = report
  return (
    <div className="daily">
      {nav}
      <div className="daily-date">{rep.date} · 生成于 {formatBeijingTime(rep.generatedAt)}</div>
      {rep.lead && (
        <section className="daily-lead">
          <h2>{rep.lead.title}</h2>
          <p>{rep.lead.leadParagraph}</p>
        </section>
      )}
      {rep.sections.map((sec) => (
        <section key={sec.label} className="daily-section">
          <h3>{sec.label}</h3>
          {sec.items.map((it, i) => (
            <div key={i} className="daily-item">
              {it.permalink ? (
                <a className="item-title" href={it.permalink}>{it.title}</a>
              ) : (
                <span className="item-title">{it.title}</span>
              )}
              {it.summary && <p className="item-summary">{it.summary}</p>}
              <span className="item-meta">{it.sourceName}</span>
            </div>
          ))}
        </section>
      ))}
      {rep.flashes.length > 0 && (
        <section className="daily-section">
          <h3>快讯</h3>
          {rep.flashes.map((f, i) => (
            <div key={i} className="daily-flash">
              {f.permalink ? <a href={f.permalink}>{f.title}</a> : <span>{f.title}</span>}
              <span className="item-meta"> {f.sourceName}{f.publishedAt ? ' · ' + formatBeijingTime(f.publishedAt) : ''}</span>
            </div>
          ))}
        </section>
      )}
    </div>
  )
}
