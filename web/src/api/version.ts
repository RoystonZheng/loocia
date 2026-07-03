export interface PublicVersion {
  apiVersion: string
  skillVersion: string
  updatedAt: string
  changelogUrl: string
  recentChanges: string[]
}

export async function fetchVersion(): Promise<PublicVersion> {
  const res = await fetch('/api/public/version')
  if (!res.ok) throw new Error(`version fetch failed: ${res.status}`)
  return (await res.json()) as PublicVersion
}
