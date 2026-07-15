import { useEffect, useState } from 'react'
import { fetchDailyList, fetchDailyByDate, type DailyReport, type DailySummary } from '../api/daily'

type State = 'loading' | 'ready' | 'empty' | 'error'

function weekday(date: string): string {
  const d = new Date(date)
  if (isNaN(d.getTime())) return ''
  return new Intl.DateTimeFormat('zh-CN', { weekday: 'long', timeZone: 'Asia/Shanghai' }).format(d)
}

// groupByMonth splits summaries (already newest-first) into {label:'2026 年 7 月', items:[]}.
function groupByMonth(list: DailySummary[]): { key: string; label: string; items: DailySummary[] }[] {
  const groups: { key: string; label: string; items: DailySummary[] }[] = []
  for (const s of list) {
    const key = s.date.slice(0, 7)
    let g = groups.find((x) => x.key === key)
    if (!g) {
      const [y, m] = key.split('-')
      g = { key, label: `${y} 年 ${Number(m)} 月`, items: [] }
      groups.push(g)
    }
    g.items.push(s)
  }
  return groups
}

export function DailyView() {
  const [state, setState] = useState<State>('loading')
  const [list, setList] = useState<DailySummary[]>([]) // newest first
  const [idx, setIdx] = useState(0)
  const [report, setReport] = useState<DailyReport | null>(null)

  useEffect(() => {
    fetchDailyList()
      .then((l) => {
        if (l.length === 0) setState('empty')
        else setList(l)
      })
      .catch(() => setState('error'))
  }, [])

  useEffect(() => {
    if (list.length === 0) return
    setState('loading')
    fetchDailyByDate(list[idx].date)
      .then((rep) => {
        if (rep === null) setState('empty')
        else {
          setReport(rep)
          setState('ready')
        }
      })
      .catch(() => setState('error'))
  }, [list, idx])

  if (state === 'error') return <p className="daily-status feed-error">加载失败，请稍后重试。</p>
  if (state === 'empty' && list.length === 0) return <p className="daily-status">暂无日报。</p>
  if (list.length === 0) return <p className="daily-status">加载中…</p>

  return (
    <div className="daily-layout">
      <aside className="daily-archive">
        <div className="daily-subnav">
          <button className="active">日报</button>
        </div>
        {groupByMonth(list).map((g) => (
          <div key={g.key} className="darc-month">
            <div className="darc-month-head">
              <span>{g.label}</span>
              <span className="darc-count">{g.items.length}</span>
            </div>
            {g.items.map((s) => {
              const active = list[idx].date === s.date
              return (
                <button
                  key={s.date}
                  className={`darc-item${active ? ' active' : ''}`}
                  onClick={() => setIdx(list.findIndex((x) => x.date === s.date))}
                >
                  <span className="darc-day">{Number(s.date.slice(8, 10))} 日</span>
                  <span className="darc-snip">{s.leadTitle}</span>
                </button>
              )
            })}
          </div>
        ))}
      </aside>

      <div className="daily-main">{report && <DailyPaper rep={report} />}</div>
    </div>
  )
}

function DailyPaper({ rep }: { rep: DailyReport }) {
  const stories = rep.sections.reduce((n, s) => n + s.items.length, 0)
  const minutes = Math.max(1, Math.round(stories * 0.7))
  const dot = rep.date.replaceAll('-', '.')

  return (
    <article className="paper">
      <header className="paper-masthead">
        <div className="paper-vol">
          <span className="paper-vol-rule" /> VOL.{dot} · {stories} STORIES · AI COOL DAILY
        </div>
        <h1 className="paper-name">
          AI <span className="paper-name-hot">Cool</span> 日报
        </h1>
        <div className="paper-dateline">
          <span>{rep.date} · {weekday(rep.date)}</span>
          <span className="paper-daily-tag">DAILY · 每早八时</span>
        </div>
      </header>

      <section className="paper-highlights">
        <div className="ph-head">
          <span>今日看点</span>
          <span className="ph-meta">{stories} 篇报道 · 约 {minutes} 分钟</span>
        </div>
        {rep.lead && <p className="ph-lead">{rep.lead.leadParagraph}</p>}
        {rep.sections.map((sec, i) => (
          <div key={sec.label} className="ph-row">
            <span className="ph-no">{String(i + 1).padStart(2, '0')}</span>
            <div className="ph-body">
              <div className="ph-cat">{sec.label}</div>
              {sec.items[0] && (
                <a className="ph-item" href={sec.items[0].permalink ?? undefined}>{sec.items[0].title}</a>
              )}
            </div>
            <span className="ph-count">{sec.items.length}</span>
          </div>
        ))}
      </section>

      {rep.sections.map((sec) => (
        <section key={sec.label} className="paper-section">
          <h3>{sec.label}</h3>
          {sec.items.map((it, i) => (
            <div key={i} className="paper-item">
              {it.permalink ? (
                <a className="paper-item-title" href={it.permalink}>{it.title}</a>
              ) : (
                <span className="paper-item-title">{it.title}</span>
              )}
              {it.summary && <p className="paper-item-summary">{it.summary}</p>}
              <span className="paper-item-src">{it.sourceName}</span>
            </div>
          ))}
        </section>
      ))}

      {rep.flashes.length > 0 && (
        <section className="paper-section">
          <h3>快讯</h3>
          {rep.flashes.map((f, i) => (
            <div key={i} className="paper-flash">
              {f.permalink ? <a href={f.permalink}>{f.title}</a> : <span>{f.title}</span>}
              <span className="paper-item-src"> {f.sourceName}</span>
            </div>
          ))}
        </section>
      )}
    </article>
  )
}
