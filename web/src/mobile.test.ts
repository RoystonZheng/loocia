/// <reference types="node" />
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

describe('mobile breakpoint', () => {
  it('index.css has a 600px breakpoint that restyles the sidebar', () => {
    const css = readFileSync(resolve(process.cwd(), 'src/index.css'), 'utf8')
    expect(css).toContain('max-width: 600px')
    const block = css.slice(css.indexOf('max-width: 600px'))
    expect(block).toContain('.sidebar')
  })
})
