import { describe, it, expect } from 'vitest'
import { formatBeijingTime, categoryLabel } from './format'

describe('formatBeijingTime', () => {
  it('renders a UTC instant in Beijing time (UTC+8)', () => {
    const s = formatBeijingTime('2026-05-07T04:00:00Z')
    expect(s).toContain('2026')
    expect(s).toContain('12:00')
  })

  it('handles an already-offset timestamp', () => {
    const s = formatBeijingTime('2026-05-07T12:00:00+08:00')
    expect(s).toContain('12:00')
  })

  it('returns empty string for undefined/empty', () => {
    expect(formatBeijingTime(undefined)).toBe('')
    expect(formatBeijingTime('')).toBe('')
  })

  it('returns the raw string if unparseable', () => {
    expect(formatBeijingTime('not-a-date')).toBe('not-a-date')
  })
})

describe('categoryLabel', () => {
  it('maps known slugs to Chinese labels', () => {
    expect(categoryLabel('ai-models')).toBe('模型发布/更新')
    expect(categoryLabel('ai-products')).toBe('产品发布/更新')
    expect(categoryLabel('industry')).toBe('行业动态')
    expect(categoryLabel('paper')).toBe('论文研究')
    expect(categoryLabel('tip')).toBe('技巧与观点')
  })

  it('passes through unknown/undefined', () => {
    expect(categoryLabel('weird')).toBe('weird')
    expect(categoryLabel(undefined)).toBe('')
  })
})
