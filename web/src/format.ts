const CATEGORY_LABELS: Record<string, string> = {
  'ai-models': '模型发布/更新',
  'ai-products': '产品发布/更新',
  industry: '行业动态',
  paper: '论文研究',
  tip: '技巧与观点',
}

// formatBeijingTime renders an ISO instant in Beijing time (Asia/Shanghai).
// Empty/undefined → ''. Unparseable → the raw input (so we never render "Invalid Date").
export function formatBeijingTime(iso: string | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return iso
  const parts = new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', hour12: false,
  }).formatToParts(d)
  const get = (t: string) => parts.find((p) => p.type === t)?.value ?? ''
  return `${get('year')}-${get('month')}-${get('day')} ${get('hour')}:${get('minute')}`
}

// categoryLabel maps a slug to its Chinese label; unknown → the slug, undefined → ''.
export function categoryLabel(slug: string | undefined): string {
  if (!slug) return ''
  return CATEGORY_LABELS[slug] ?? slug
}
