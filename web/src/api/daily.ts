export interface DailySectionItem {
  title: string
  summary: string
  sourceUrl: string
  sourceName: string
  permalink?: string | null
}

export interface DailySection {
  label: string
  items: DailySectionItem[]
}

export interface DailyFlash {
  title: string
  sourceName: string
  sourceUrl: string
  publishedAt?: string | null
  permalink?: string | null
}

export interface DailyReport {
  date: string
  generatedAt: string
  windowStart: string
  windowEnd: string
  lead: { title: string; leadParagraph: string } | null
  sections: DailySection[]
  flashes: DailyFlash[]
}

// DailySummary is one entry in the archive list (date + lead only, no sections).
export interface DailySummary {
  date: string
  generatedAt: string
  leadTitle: string
  leadParagraph: string
}

// fetchLatestDaily returns the latest report, or null when none exists (404).
export async function fetchLatestDaily(): Promise<DailyReport | null> {
  const res = await fetch('/api/public/daily')
  if (res.status === 404) return null
  if (!res.ok) throw new Error(`daily fetch failed: ${res.status}`)
  return (await res.json()) as DailyReport
}

// fetchDailyList returns the archive index (dates, newest first), [] when empty.
export async function fetchDailyList(): Promise<DailySummary[]> {
  const res = await fetch('/api/public/dailies')
  if (!res.ok) throw new Error(`dailies fetch failed: ${res.status}`)
  const body = (await res.json()) as { items?: DailySummary[] }
  return body.items ?? []
}

// fetchDailyByDate returns one day's full report, or null when absent (404).
export async function fetchDailyByDate(date: string): Promise<DailyReport | null> {
  const res = await fetch(`/api/public/daily/${date}`)
  if (res.status === 404) return null
  if (!res.ok) throw new Error(`daily ${date} fetch failed: ${res.status}`)
  return (await res.json()) as DailyReport
}
