import { describe, it, expect } from 'vitest'
import { formatBeijingTime, categoryLabel, scoreLabel, scoreTier } from './format'

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

describe('scoreLabel', () => {
  it('maps 1-5 to D..S', () => {
    expect(scoreLabel(5)).toBe('S')
    expect(scoreLabel(4)).toBe('A')
    expect(scoreLabel(3)).toBe('B')
    expect(scoreLabel(2)).toBe('C')
    expect(scoreLabel(1)).toBe('D')
  })
  it('clamps out-of-range and handles null', () => {
    expect(scoreLabel(9)).toBe('S')
    expect(scoreLabel(0)).toBe('D')
    expect(scoreLabel(null)).toBe('')
    expect(scoreLabel(undefined)).toBe('')
  })
})

describe('scoreTier', () => {
  it('returns the same letter as scoreLabel for valid input', () => {
    expect(scoreTier(5)).toBe('S')
    expect(scoreTier(1)).toBe('D')
  })
})
