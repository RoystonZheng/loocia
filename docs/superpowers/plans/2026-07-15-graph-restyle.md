# 图谱改版（宋体墨色 + 子图动效）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 词云换宋体纯墨皮肤（热度 4 档墨阶）；子图修字体重叠 + 焦点切换平滑过渡 + hover 聚焦调光；连同已提交的详情页侧栏修复一起部署。

**Architecture:** 纯前端为主：新增共享墨阶模块 `ink.ts`；`graphLayout` 加位置种子和标签感知碰撞；`TermPanel` 改为 transform 定位 + CSS 过渡的持久节点渲染；CSS 新增 `--gv-paper`/`--gv-ink-1..4` 双主题 token。后端零改动（detailpage 修复已在 6f0fd62）。

**Tech Stack:** 现有栈；无新依赖。

**Spec:** `docs/superpowers/specs/2026-07-15-graph-restyle-design.md`

**铁律：** 前端命令在 `web/` 下跑；测试用 fireEvent；提交署名 `git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit`；只动列出的文件（并行会话可能在 server/ 干活）。

## File Structure

- Create: `web/src/ink.ts` + `web/src/ink.test.ts` — 墨阶分档 + 标签估宽（GraphView/TermPanel/graphLayout 共用）
- Modify: `web/src/graphLayout.ts` + `graphLayout.test.ts` — seed 参数、标签感知碰撞/clamp
- Modify: `web/src/cloudLayout.ts` — CloudWord 加 `tier` 字段（透传）
- Modify: `web/src/components/GraphView.tsx`（+ 其测试若断言受影响）— 墨阶类名替换 entity/topic
- Modify: `web/src/components/TermPanel.tsx` + `TermPanel.test.tsx` — 平滑过渡/hover/halo/墨阶
- Modify: `web/src/index.css` — token + 全部 gv-* 样式改版

---

### Task 1: ink.ts 共享模块（墨阶 + 标签估宽）

**Files:**
- Create: `web/src/ink.ts`
- Test: `web/src/ink.test.ts`

- [ ] **Step 1: 写失败测试**

`web/src/ink.test.ts`:

```ts
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
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd /Users/didi/aihot-internal/web && npx vitest run src/ink.test.ts`
Expected: FAIL（模块不存在）

- [ ] **Step 3: 实现**

`web/src/ink.ts`:

```ts
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
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd /Users/didi/aihot-internal/web && npx vitest run src/ink.test.ts` → PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/ink.ts web/src/ink.test.ts
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(web): 墨阶分档 + 标签估宽共享模块"
```

---

### Task 2: graphLayout 升级（seed + 标签感知碰撞/clamp）

**Files:**
- Modify: `web/src/graphLayout.ts`
- Test: `web/src/graphLayout.test.ts`（追加）

- [ ] **Step 1: 追加失败测试**（保留现有 2 个测试）

在 `graphLayout.test.ts` 追加：

```ts
import { labelHalfWidth } from './ink'

describe('layoutGraph seeded', () => {
  it('keeps seeded nodes near their previous positions', () => {
    const nodes = [
      { id: 'focus', r: 26 },
      { id: 'a', r: 14 },
      { id: 'b', r: 10 },
    ]
    const edges = [
      { source: 'focus', target: 'a', width: 3 },
      { source: 'focus', target: 'b', width: 1 },
    ]
    const first = layoutGraph(nodes, edges, 400, 300)
    const seed = new Map(first.nodes.map((n) => [n.id, { x: n.x, y: n.y }]))
    const second = layoutGraph(nodes, edges, 400, 300, seed)
    for (const n of second.nodes) {
      const prev = seed.get(n.id)!
      const dist = Math.hypot(n.x - prev.x, n.y - prev.y)
      expect(dist).toBeLessThan(40) // 收敛过的种子不应大幅漂移
    }
  })

  it('clamps within label-aware margins', () => {
    const nodes = [{ id: '很长很长很长的一个标签词', r: 8 }]
    const { nodes: placed } = layoutGraph(nodes, [], 200, 120)
    const m = Math.max(8, labelHalfWidth('很长很长很长的一个标签词'))
    expect(placed[0].x).toBeGreaterThanOrEqual(m)
    expect(placed[0].x).toBeLessThanOrEqual(200 - m)
    expect(placed[0].y).toBeLessThanOrEqual(120 - (8 + 16)) // 下方留标签位
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd /Users/didi/aihot-internal/web && npx vitest run src/graphLayout.test.ts`
Expected: seeded 测试因签名不接受第 5 参而编译失败（或 clamp 断言失败）

- [ ] **Step 3: 实现**

`web/src/graphLayout.ts` 变更点（其余不动）：

```ts
import { labelHalfWidth } from './ink'
```

签名与种子（替换现有函数头和 ns 构造、collide、clamp 三处）：

```ts
// layoutGraph runs a force simulation synchronously (fixed ticks, no timers) —
// deterministic enough for a static render and jsdom-safe (pure math, no DOM).
// When `seed` carries previous positions, matching nodes start there with a
// lowered alpha so the new layout stays visually continuous (smooth-transition
// animation relies on this).
export function layoutGraph(
  nodes: GraphNode[],
  edges: GraphEdge[],
  width: number,
  height: number,
  seed?: Map<string, { x: number; y: number }>,
): { nodes: PlacedNode[]; edges: PlacedEdge[] } {
  const ns: SimNode[] = nodes.map((n) => {
    const s = seed?.get(n.id)
    return s ? { ...n, x: s.x, y: s.y } : { ...n }
  })
  const ls: (SimulationLinkDatum<SimNode> & { width: number })[] = edges.map((e) => ({ ...e }))

  const sim = forceSimulation(ns)
    .force('link', forceLink(ls).id((d: SimulationNodeDatum) => (d as SimNode).id).distance(90))
    .force('charge', forceManyBody().strength(-160))
    .force('center', forceCenter(width / 2, height / 2))
    .force('collide', forceCollide<SimNode>().radius((d) => Math.max(d.r, labelHalfWidth(d.id)) + 10))
    .stop()
  if (seed && seed.size > 0) sim.alpha(0.3) // 种子已接近平衡，别把它踢飞
  for (let i = 0; i < 200; i++) sim.tick()

  const marginX = (n: SimNode) => Math.max(n.r, labelHalfWidth(n.id))
  const clampX = (n: SimNode) => Math.min(Math.max(n.x ?? 0, marginX(n)), width - marginX(n))
  const clampY = (n: SimNode) => Math.min(Math.max(n.y ?? 0, n.r), height - (n.r + 16))
  ...（placed/byID/lines 不变）
```

- [ ] **Step 4: 全部 graphLayout 测试通过 + typecheck**

Run: `cd /Users/didi/aihot-internal/web && npx vitest run src/graphLayout.test.ts && npx tsc -b` → PASS
（若 seeded 漂移断言不稳，允许把 40 放宽到 60 并注明；不许删断言。）

- [ ] **Step 5: Commit**

```bash
git add web/src/graphLayout.ts web/src/graphLayout.test.ts
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(web): 布局支持位置种子 + 标签感知碰撞与边界"
```

---

### Task 3: 词云皮肤（GraphView + cloudLayout + CSS token）

**Files:**
- Modify: `web/src/cloudLayout.ts`（CloudWord 加 tier）
- Modify: `web/src/components/GraphView.tsx`
- Modify: `web/src/index.css`（token + 词云段）
- Test: 现有 `GraphView.test.tsx` 全保；mock 已经 `...w` 透传 tier，无需改

- [ ] **Step 1: cloudLayout.ts** — `CloudWord` 接口加一行 `tier: number`（PlacedWord extends 自动获得）。

- [ ] **Step 2: GraphView.tsx**：

```ts
import { inkTier } from '../ink'
```
- effect 里 map 处改为：
```ts
res.terms.map((t: CloudTerm) => ({
  text: t.term,
  size: fontSize(t.count, maxCount),
  kind: t.kind,
  tier: inkTier(t.count, maxCount),
})),
```
- `<text>` 的 className 改为：
```tsx
className={`gv-word gv-ink-t${w.tier}${selected === w.text ? ' selected' : ''}`}
```
（kind 字段保留在数据里但不再参与视觉。）

- [ ] **Step 3: index.css** —
1. 在现有亮色 token 块（`--accent` 等旁边）加：
```css
  --gv-paper: #faf9f6;
  --gv-ink-1: #1a1a1a;
  --gv-ink-2: #3d3d3b;
  --gv-ink-3: #6a6a63;
  --gv-ink-4: #98988f;
```
2. 在现有暗色 token 块（找到定义 `--bg:#0f1115` 的那个选择器，两处都要：`data-theme` 和跟随系统的 media query 若分开写）加：
```css
  --gv-paper: #16181d;
  --gv-ink-1: #e8e8e4;
  --gv-ink-2: #c9c9c4;
  --gv-ink-3: #9a9a92;
  --gv-ink-4: #6b6b64;
```
3. 词云段替换（原 `.gv-cloud`/`.gv-word` 相关规则）：
```css
.gv-cloud {
  width: 100%; height: auto; display: block;
  background: var(--gv-paper); border: 1px solid var(--border); border-radius: 10px;
  animation: gv-fade .3s ease both;
}
@keyframes gv-fade { from { opacity: 0; } to { opacity: 1; } }
.gv-word { cursor: pointer; font-family: var(--serif); }
.gv-ink-t1 { fill: var(--gv-ink-1); font-weight: 900; }
.gv-ink-t2 { fill: var(--gv-ink-2); font-weight: 700; }
.gv-ink-t3 { fill: var(--gv-ink-3); font-weight: 400; }
.gv-ink-t4 { fill: var(--gv-ink-4); font-weight: 400; }
.gv-word:hover, .gv-word.selected { opacity: .7; text-decoration: underline; }
```
删掉旧的 `.gv-word.entity` / `.gv-word.topic` 两条。

- [ ] **Step 4: 验证 + Commit**

```bash
cd /Users/didi/aihot-internal/web && npx vitest run && npx tsc -b && npm run lint
git add web/src/cloudLayout.ts web/src/components/GraphView.tsx web/src/index.css
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(web): 词云宋体纯墨皮肤（4 档墨阶 + 纸底）"
```

---

### Task 4: TermPanel 平滑过渡 + hover + 防重叠

**Files:**
- Modify: `web/src/components/TermPanel.tsx`
- Modify: `web/src/components/TermPanel.test.tsx`（追加 3 个测试）
- Modify: `web/src/index.css`（子图段）

- [ ] **Step 1: 追加失败测试**（现有 4 个测试保留不动；新增依赖第二种 payload）

在 `TermPanel.test.tsx` 追加（顶部补 `import { waitFor } from '@testing-library/react'` 已有则略）：

```tsx
const termPayload2 = {
  term: '推理模型', kind: 'topic', count: 18,
  neighbors: [
    { term: 'OpenAI', kind: 'entity', weight: 9 },
    { term: '芯片', kind: 'topic', weight: 2 },
  ],
  items: [
    { id: 'i2', title: '另一条', url: 'https://x/i2', permalink: '/items/i2', source: 'S2', selected: false },
  ],
}

function mockTwoTerms() {
  globalThis.fetch = vi.fn().mockImplementation((url: string) => {
    const u = decodeURIComponent(String(url))
    if (u.includes('/graph/term/推理模型')) {
      return Promise.resolve({ ok: true, json: async () => termPayload2 })
    }
    return Promise.resolve({ ok: true, json: async () => termPayload })
  }) as unknown as typeof fetch
}

it('keeps shared nodes mounted across a focus switch (smooth transition)', async () => {
  mockTwoTerms()
  const { rerender } = render(<TermPanel term="OpenAI" window="7d" onSelect={() => {}} />)
  await waitFor(() => expect(screen.getByText('推理模型')).toBeInTheDocument())
  const before = screen.getByText('OpenAI').closest('g')
  rerender(<TermPanel term="推理模型" window="7d" onSelect={() => {}} />)
  await waitFor(() => expect(screen.getByText('芯片')).toBeInTheDocument())
  // OpenAI 在两轮里都存在（先焦点后邻居）：DOM 节点必须是同一个（没被重建）
  expect(screen.getByText('OpenAI').closest('g')).toBe(before)
})

it('fades out and removes exiting nodes after the transition', async () => {
  vi.useFakeTimers()
  try {
    mockTwoTerms()
    const { rerender } = render(<TermPanel term="OpenAI" window="7d" onSelect={() => {}} />)
    await vi.waitFor(() => expect(screen.getByText('开源')).toBeInTheDocument())
    rerender(<TermPanel term="推理模型" window="7d" onSelect={() => {}} />)
    await vi.waitFor(() => expect(screen.getByText('芯片')).toBeInTheDocument())
    // 「开源」不在新邻居里：先带 exiting 类，计时器走完后从 DOM 消失
    expect(screen.getByText('开源').closest('g')!.className.baseVal).toContain('exiting')
    vi.advanceTimersByTime(500)
    expect(screen.queryByText('开源')).toBeNull()
  } finally {
    vi.useRealTimers()
  }
})

it('dims非 hover 的节点', async () => {
  globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => termPayload }) as unknown as typeof fetch
  render(<TermPanel term="OpenAI" window="7d" onSelect={() => {}} />)
  await waitFor(() => expect(screen.getByText('推理模型')).toBeInTheDocument())
  fireEvent.mouseEnter(screen.getByText('推理模型').closest('g')!)
  expect(screen.getByText('开源').closest('g')!.className.baseVal).toContain('dim')
  expect(screen.getByText('推理模型').closest('g')!.className.baseVal).not.toContain('dim')
  fireEvent.mouseLeave(screen.getByText('推理模型').closest('g')!)
  expect(screen.getByText('开源').closest('g')!.className.baseVal).not.toContain('dim')
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd /Users/didi/aihot-internal/web && npx vitest run src/components/TermPanel.test.tsx`
Expected: 新 3 个 FAIL（旧 4 个 PASS）

- [ ] **Step 3: 重写 TermPanel.tsx**

```tsx
import { useEffect, useRef, useState } from 'react'
import { fetchTerm, type GraphWindow, type TermResponse } from '../api/graph'
import { layoutGraph, type GraphEdge, type GraphNode, type PlacedNode } from '../graphLayout'
import { inkTier } from '../ink'
import { formatBeijingTime } from '../format'

const W = 420
const H = 320
const FOCUS_R = 26
const EXIT_MS = 400

interface Rendered extends PlacedNode {
  tier: 1 | 2 | 3 | 4
  isFocus: boolean
  entering: boolean
  exiting: boolean
}

// TermPanel is the drill-down under the cloud: an ink-styled co-occurrence
// subgraph with staged transitions (shared nodes slide, leavers fade out,
// newcomers fade in) + the term's newest items.
export function TermPanel({
  term, window: win, onSelect,
}: {
  term: string
  window: GraphWindow
  onSelect: (term: string) => void
}) {
  const [data, setData] = useState<TermResponse | null>(null)
  const [failed, setFailed] = useState(false)
  const [hovered, setHovered] = useState<string | null>(null)
  const [exiting, setExiting] = useState<Rendered[]>([])
  // positions survive data updates so the next layout can seed from them
  const positions = useRef(new Map<string, { x: number; y: number }>())

  useEffect(() => {
    let stale = false
    setData(null)
    setFailed(false)
    setHovered(null)
    fetchTerm(term, win)
      .then((d) => { if (!stale) setData(d) })
      .catch(() => { if (!stale) setFailed(true) })
    return () => { stale = true }
  }, [term, win])

  // exiting nodes leave the DOM after their fade-out completes
  useEffect(() => {
    if (exiting.length === 0) return
    const t = setTimeout(() => setExiting([]), EXIT_MS)
    return () => clearTimeout(t)
  }, [exiting])

  if (failed) return <div className="gv-panel gv-status">加载失败，稍后再试</div>
  if (!data) return <div className="gv-panel gv-status">加载中…</div>
  if (data.count === 0) return <div className="gv-panel gv-status">该时间段暂无数据</div>

  const maxW = Math.max(...data.neighbors.map((n) => n.weight), 1)
  const nodes: GraphNode[] = [
    { id: data.term, r: FOCUS_R },
    ...data.neighbors.map((n) => ({ id: n.term, r: 10 + 12 * (n.weight / maxW) })),
  ]
  const edges: GraphEdge[] = data.neighbors.map((n) => ({
    source: data.term, target: n.term, width: 1 + 3 * (n.weight / maxW),
  }))
  const prev = positions.current
  const g = layoutGraph(nodes, edges, W, H, prev)

  const tierOf = new Map<string, 1 | 2 | 3 | 4>([
    [data.term, 1],
    ...data.neighbors.map((n) => [n.term, inkTier(n.weight, maxW)] as const),
  ])
  const rendered: Rendered[] = g.nodes.map((n) => ({
    ...n,
    tier: tierOf.get(n.id) ?? 4,
    isFocus: n.id === data.term,
    entering: !prev.has(n.id),
    exiting: false,
  }))

  // stage leavers exactly once per layout change (render-phase state derivation)
  const currentIDs = new Set(g.nodes.map((n) => n.id))
  const leavers: Rendered[] = []
  for (const [id, p] of prev) {
    if (!currentIDs.has(id) && !exiting.some((e) => e.id === id)) {
      leavers.push({ id, r: 10, x: p.x, y: p.y, tier: 4, isFocus: false, entering: false, exiting: true })
    }
  }
  positions.current = new Map(g.nodes.map((n) => [n.id, { x: n.x, y: n.y }]))
  if (leavers.length > 0) {
    // schedule after render to avoid setState-in-render warnings
    queueMicrotask(() => setExiting((old) => [...old, ...leavers]))
  }

  const edgeTargets = data.neighbors.map((n) => n.term)

  return (
    <section className="gv-panel">
      <div className="gv-subgraph">
        <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={`${data.term} 关联图`}>
          <g key={data.term} className="gv-edges">
            {g.edges.map((e, i) => (
              <line
                key={edgeTargets[i]}
                x1={e.x1} y1={e.y1} x2={e.x2} y2={e.y2}
                className={`gv-edge${hovered && hovered !== edgeTargets[i] ? ' dim' : ''}${hovered === edgeTargets[i] ? ' lit' : ''}`}
                strokeWidth={e.width}
              />
            ))}
          </g>
          {exiting.map((n) => (
            <g key={`exit-${n.id}`} className="gv-node exiting" transform={`translate(${n.x},${n.y})`}>
              <circle r={n.r} />
              <text y={n.r + 12} textAnchor="middle">{n.id}</text>
            </g>
          ))}
          {rendered.map((n) => (
            <g
              key={n.id}
              className={[
                'gv-node',
                `gv-ink-t${n.tier}`,
                n.isFocus ? 'focus' : '',
                n.entering ? 'enter' : '',
                hovered && hovered !== n.id && !n.isFocus ? 'dim' : '',
              ].filter(Boolean).join(' ')}
              transform={`translate(${n.x},${n.y})`}
              onClick={() => { if (!n.isFocus) onSelect(n.id) }}
              onMouseEnter={() => { if (!n.isFocus) setHovered(n.id) }}
              onMouseLeave={() => setHovered(null)}
            >
              <circle r={n.r} />
              <text y={n.r + 12} textAnchor="middle">{n.id}</text>
            </g>
          ))}
        </svg>
      </div>
      <div className="gv-items">
        <h3>「{data.term}」 · {data.count} 条相关</h3>
        {data.items.map((it) => (
          <div key={it.id} className="gv-item-row">
            <a href={it.permalink}>{it.title}</a>
            <span className="gv-item-meta">
              {it.source}
              {it.publishedAt ? ` · ${formatBeijingTime(it.publishedAt)}` : ''}
            </span>
          </div>
        ))}
      </div>
    </section>
  )
}
```

实现注意：
- 若 `queueMicrotask` + setState 造成测试环境 act 警告，可改成 `useEffect` 按 `data` 依赖收集 leavers（把 prev 快照存 ref），行为等价即可——测试断言的是"exiting 类出现 → 500ms 后消失"。
- kindOf/entity/topic 类名整体移除。

- [ ] **Step 4: index.css 子图段替换**

替换现有 `.gv-subgraph`/`.gv-edge`/`.gv-node` 段为：

```css
.gv-subgraph svg {
  width: 100%; height: auto;
  background: var(--gv-paper); border: 1px solid var(--border); border-radius: 10px;
}
.gv-edge { stroke: var(--rail); transition: opacity .25s ease, stroke .25s ease; }
.gv-edges { animation: gv-fade .4s ease both; }
.gv-edge.dim { opacity: .25; }
.gv-edge.lit { stroke: var(--gv-ink-2); }
.gv-node { cursor: pointer; transition: transform .6s cubic-bezier(.4,0,.2,1), opacity .4s ease; }
.gv-node.focus { cursor: default; }
.gv-node.enter { animation: gv-fade .4s ease both; }
.gv-node.exiting { opacity: 0; pointer-events: none; }
.gv-node.dim { opacity: .3; }
.gv-node.gv-ink-t1 circle { fill: var(--gv-ink-1); }
.gv-node.gv-ink-t2 circle { fill: var(--gv-ink-2); }
.gv-node.gv-ink-t3 circle { fill: var(--gv-ink-3); }
.gv-node.gv-ink-t4 circle { fill: var(--gv-ink-4); }
.gv-node text {
  fill: var(--text); font-size: 12px; font-family: var(--serif);
  paint-order: stroke; stroke: var(--gv-paper); stroke-width: 3px;
}
@media (prefers-reduced-motion: reduce) {
  .gv-node, .gv-edge { transition: none; }
  .gv-edges, .gv-node.enter, .gv-cloud { animation: none; }
}
```
删掉旧的 `.gv-node.entity circle` / `.gv-node.topic circle` / `.gv-node.focus circle` 三条（focus 的辨识靠尺寸+最深墨阶）。

- [ ] **Step 5: 全量验证 + Commit**

```bash
cd /Users/didi/aihot-internal/web && npx vitest run && npx tsc -b && npm run lint && npm run build
git add web/src/components/TermPanel.tsx web/src/components/TermPanel.test.tsx web/src/index.css
git -c user.name='Claude' -c user.email='noreply@anthropic.com' commit -m "feat(web): 子图平滑过渡 + hover 调光 + 标签光晕防重叠"
```

---

### Task 5: 部署 + 线上人工验收

- [ ] **Step 1:** `cd web && npm run build`；交叉编译 server（带 6f0fd62 detailpage 变更）：`cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o <scratch>/server .`
- [ ] **Step 2:** scp `server.new` → chmod → `mv -f` 原子替换 → `supervisorctl restart aihot-server`；`rsync web/dist/ melos:/var/www/aihot/`
- [ ] **Step 3:** 冒烟：`curl http://10.190.12.242:8899/api/public/graph/cloud?window=7d` 正常；`curl http://10.190.12.242:8899/items/<某id> | grep '/#graph'` 侧栏含图谱项
- [ ] **Step 4:** 请用户浏览器验收：亮/暗主题词云观感、点词→子图平滑过渡、hover 调光、标签不重叠、从详情页侧栏回图谱

## Self-Review 结论（已跑）

- Spec 覆盖：墨阶 token/分档(T1/T3)、宋体+纸底(T3/T4)、防重叠三招(T2 碰撞/边界、T4 光晕)、平滑过渡+hover+reduced-motion(T4)、侧栏修复部署(T5)。无 TBD。
- 类型一致：`inkTier` 返回 1|2|3|4 与 `Rendered.tier` 对齐；`layoutGraph` 新参数向后兼容（GraphView 不传 seed 不受影响，但 cloudLayout 的 CloudWord 加了必填 tier —— GraphView 是唯一调用方，T3 同步改）。
- 已知风险标注：seeded 漂移阈值(40px)可放宽到 60；queueMicrotask setState 若报 act 警告有等价替代方案，都写在任务里。
