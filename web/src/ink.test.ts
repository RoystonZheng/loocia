import { describe, it, expect } from 'vitest'
import { inkTier, labelHalfWidth } from './ink'

describe('inkTier', () => {
  it('maps ratio to 4 tiers with the spec thresholds', () => {
    expect(inkTier(100, 100)).toBe(1) // 1.0
    expect(inkTier(66, 100)).toBe(1)  // 0.66 边界含
    expect(inkTier(65, 100)).toBe(2)
    expect(inkTier(40, 100)).toBe(2)  // 0.4 边界含
    expect(inkTier(39, 100)).toBe(3)
    expect(inkTier(18, 100)).toBe(3)  // 0.18 边界含
    expect(inkTier(17, 100)).toBe(4)
    expect(inkTier(1, 100)).toBe(4)
  })
  it('never divides by zero', () => {
    expect(inkTier(0, 0)).toBe(1)
  })
})

describe('labelHalfWidth', () => {
  it('counts CJK chars at full fontSize and latin at 0.62', () => {
    expect(labelHalfWidth('推理', 12)).toBeCloseTo(12)          // 2*12/2
    expect(labelHalfWidth('AI', 12)).toBeCloseTo(7.44)          // 2*7.44/2
    expect(labelHalfWidth('AI安全', 12)).toBeCloseTo(19.44)     // (2*7.44+2*12)/2
  })
  it('defaults fontSize to 12', () => {
    expect(labelHalfWidth('推理')).toBeCloseTo(12)
  })
})
