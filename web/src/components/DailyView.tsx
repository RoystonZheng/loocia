import { useEffect, useState } from 'react'
import { fetchDailyList, fetchDailyByDate, type DailyReport, type DailySummary } from '../api/daily'
import { WordCloud } from './WordCloud'

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

// 手机端日期条上显示的当前日期，如「7 月 16 日 · 周四」
function fmtBar(date: string): string {
  const wd = weekday(date)
  return `${Number(date.slice(5, 7))} 月 ${Number(date.slice(8, 10))} 日${wd ? ' · ' + wd : ''}`
}

// 往期日期列表（桌面左栏 + 手机下拉复用同一份，避免重复）
function ArchiveList({ list, idx, onPick }: { list: DailySummary[]; idx: number; onPick: (i: number) => void }) {
  return (
    <>
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
                onClick={() => onPick(list.findIndex((x) => x.date === s.date))}
              >
                <span className="darc-day">{Number(s.date.slice(8, 10))} 日</span>
                <span className="darc-snip">{s.leadTitle}</span>
              </button>
            )
          })}
        </div>
      ))}
    </>
  )
}

export function DailyView() {
  const [state, setState] = useState<State>('loading')
  const [list, setList] = useState<DailySummary[]>([]) // newest first
  const [idx, setIdx] = useState(0)
  const [report, setReport] = useState<DailyReport | null>(null)
  const [archiveOpen, setArchiveOpen] = useState(false) // 手机端「往期」下拉开合

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
      {/* 手机端：顶部日期条 + 往期下拉（桌面隐藏） */}
      <div className="daily-datebar">
        <span className="ddb-current">{fmtBar(list[idx].date)}</span>
        <button
          className={`ddb-toggle${archiveOpen ? ' open' : ''}`}
          onClick={() => setArchiveOpen((o) => !o)}
          aria-expanded={archiveOpen}
        >
          往期 <span className="ddb-caret">▾</span>
        </button>
        {archiveOpen && (
          <>
            <div className="ddb-backdrop" onClick={() => setArchiveOpen(false)} />
            <div className="ddb-dropdown">
              <ArchiveList list={list} idx={idx} onPick={(i) => { setIdx(i); setArchiveOpen(false) }} />
            </div>
          </>
        )}
      </div>

      {/* 桌面端：左侧往期栏（手机隐藏） */}
      <aside className="daily-archive">
        <div className="daily-subnav">
          <button className="active">日报</button>
        </div>
        <ArchiveList list={list} idx={idx} onPick={setIdx} />
      </aside>

      <div className="daily-main">{report && <DailyPaper rep={report} />}</div>
    </div>
  )
}

// TOC_MAX bounds headlines per section in the 今日看点 card; the rest collapse
// into a clickable …等N篇 tail that jumps to the full section.
const TOC_MAX = 3

// Plain smooth scroll — the URL hash belongs to the view router (#daily/#all/
// #graph), so anchors must not touch it.
function jumpToSection(i: number) {
  document.getElementById(`daily-sec-${i}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
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
              <button type="button" className="ph-cat" onClick={() => jumpToSection(i)}>{sec.label}</button>
              {sec.items.slice(0, TOC_MAX).map((it, j) => (
                <a key={j} className="ph-item" href={it.permalink ?? undefined}>{it.title}</a>
              ))}
              {sec.items.length > TOC_MAX && (
                <button type="button" className="ph-more" onClick={() => jumpToSection(i)}>
                  …等 {sec.items.length} 篇
                </button>
              )}
            </div>
            <span className="ph-count">{sec.items.length}</span>
          </div>
        ))}
      </section>

      <section className="paper-cloud">
        <h3>今日热词</h3>
        <p className="paper-cloud-hint">词越大越热，点一个词看当天跟它相关的内容</p>
        <WordCloud date={rep.date} height={300} />
      </section>

      {rep.sections.map((sec, i) => (
        <section key={sec.label} id={`daily-sec-${i}`} className="paper-section">
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
