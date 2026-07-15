// Shared ink-scale helpers for the graph view's 宋体墨色 skin.

// inkTier buckets a count against the window max into the 4 ink shades
// (1 = darkest/boldest). Thresholds from the restyle spec.
export function inkTier(count: number, maxCount: number): 1 | 2 | 3 | 4 {
  const ratio = maxCount > 0 ? count / maxCount : 1
  if (ratio >= 0.66) return 1
  if (ratio >= 0.4) return 2
  if (ratio >= 0.18) return 3
  return 4
}

// labelHalfWidth estimates half the rendered width of a node label:
// CJK chars ≈ fontSize wide, latin/digits ≈ 0.62 × fontSize.
export function labelHalfWidth(text: string, fontSize = 12): number {
  let w = 0
  for (const ch of text) {
    w += ch.charCodeAt(0) > 0x2e7f ? fontSize : fontSize * 0.62
  }
  return w / 2
}
