export interface HotTopic {
  id: string
  title: string
  url: string
  permalink: string
  source: string
  sourceCount: number
  sourceNames: string[]
  latestAt: string
}

export interface HotTopicList {
  count: number
  items: HotTopic[]
}

export async function fetchHotTopics(): Promise<HotTopicList> {
  const res = await fetch('/api/public/hot-topics')
  if (!res.ok) throw new Error(`hot-topics fetch failed: ${res.status}`)
  return (await res.json()) as HotTopicList
}
