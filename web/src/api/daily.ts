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

// fetchLatestDaily returns the latest report, or null when none exists (404).
export async function fetchLatestDaily(): Promise<DailyReport | null> {
  const res = await fetch('/api/public/daily')
  if (res.status === 404) return null
  if (!res.ok) throw new Error(`daily fetch failed: ${res.status}`)
  return (await res.json()) as DailyReport
}
