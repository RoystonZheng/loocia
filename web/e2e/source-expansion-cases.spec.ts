import { expect, request as playwrightRequest, test, type Page, type Route } from '@playwright/test'
import { execFile } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import { promisify } from 'node:util'

const execFileAsync = promisify(execFile)

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const webDir = path.resolve(__dirname, '..')
const repoRoot = path.resolve(webDir, '..')
const serverDir = path.join(repoRoot, 'server')

type PublicItem = {
  id: string
  title: string
  url: string
  permalink: string
  source: string
  selected: boolean
  category?: string
  score?: number
  summary?: string
  source_role?: string
}

test.describe('AI Cool 第四部分：信息源扩展 Playwright 验收', () => {
  test('TC-SRC/ADP/DEDUP/DB/RUN/PIPE/API 后端闭环：Playwright 调度相关 Go 测试', async () => {
    test.setTimeout(180_000)

    const result = await runCommand('go', [
      'test',
      '-p=1',
      './internal/pulse',
      './internal/ingest',
      './internal/pipeline',
      './internal/publicapi',
      '-count=1',
      '-v',
    ], serverDir)

    await test.info().attach('go-source-expansion-tests.txt', {
      body: result.output,
      contentType: 'text/plain',
    })
    expect(result.output).toContain('PASS')
    for (const expected of [
      'TestLoadSourcesDefaults',
      'TestAIHOTSourceMapsOriginalAndSecondaryItems',
      'TestCanonicalURLNormalizesCommonArticleVariants',
      'TestBuildPromptIncludesSourceContext',
      'TestToItemSelectionByRelevanceAndScore',
      'TestParseSourceKind',
      'TestToPublicExposesOnlyPublicFields',
    ]) {
      expect(result.output).toContain(expected)
    }
  })

  test('TC-FE-001~TC-FE-005 前端来源筛选：全部/RSS/HTML/MP/AIHOT 与错误态', async ({ page }) => {
    const calls = await installSourceExpansionRoutes(page)

    await test.step('打开全部动态并展示来源分组', async () => {
      await page.goto('/#all')
      await expect(page.getByRole('heading', { name: '全部 AI 动态' })).toBeVisible()
      for (const label of ['全部来源', 'RSS/Atom', '网页直采', '公众号', 'AIHOT补漏']) {
        await expect(page.getByRole('button', { name: label })).toBeVisible()
      }
      await expect(page.getByRole('link', { name: '全部来源汇总' })).toBeVisible()
      expect(calls.items.at(-1)).not.toContain('source_kind=')
    })

    await test.step('点击 RSS/Atom 后请求 source_kind=rss 并重置列表', async () => {
      await page.getByRole('button', { name: 'RSS/Atom' }).click()
      await expect(page.getByRole('link', { name: 'RSS 官方资讯' })).toBeVisible()
      await expect(page.getByRole('link', { name: '全部来源汇总' })).toBeHidden()
      expect(calls.items.at(-1)).toContain('source_kind=rss')
    })

    await test.step('点击网页直采后请求 source_kind=html', async () => {
      await page.getByRole('button', { name: '网页直采' }).click()
      await expect(page.getByRole('link', { name: 'HTML 网页直采资讯' })).toBeVisible()
      expect(calls.items.at(-1)).toContain('source_kind=html')
    })

    await test.step('点击公众号后请求 source_kind=mp', async () => {
      await page.getByRole('button', { name: '公众号' }).click()
      await expect(page.getByRole('link', { name: '公众号语料资讯' })).toBeVisible()
      await expect(page.getByRole('link', { name: 'HTML 网页直采资讯' })).toBeHidden()
      expect(calls.items.at(-1)).toContain('source_kind=mp')
    })

    await test.step('点击 AIHOT 补漏后请求 source_kind=aihot，且不展示内部 source_role', async () => {
      await page.getByRole('button', { name: 'AIHOT补漏' }).click()
      await expect(page.getByRole('link', { name: 'AIHOT 二手补漏资讯' })).toBeVisible()
      await expect(page.getByText('AIHOT · 二手线索')).toBeVisible()
      await expect(page.getByText('discovery')).toBeHidden()
      expect(calls.items.at(-1)).toContain('source_kind=aihot')
    })

    await test.step('接口失败时展示错误态', async () => {
      calls.failItems = true
      await page.getByRole('button', { name: '全部来源' }).click()
      await expect(page.getByText('加载失败，请稍后重试。')).toBeVisible()
    })
  })

  test('TC-E2E-003 默认外部源可达性：Playwright API live smoke', async () => {
    test.skip(process.env.AICOOL_LIVE_SOURCE_TESTS !== '1', 'set AICOOL_LIVE_SOURCE_TESTS=1 to run live external-source smoke checks')
    test.setTimeout(90_000)

    const checks = [
      { name: 'Anthropic News HTML', url: 'https://www.anthropic.com/news', statuses: [200] },
      { name: 'Google DeepMind RSS', url: 'https://deepmind.google/blog/rss.xml', statuses: [200] },
      { name: 'Cloudflare AI RSS', url: 'https://blog.cloudflare.com/tag/ai/rss/', statuses: [200] },
      { name: 'NVIDIA Generative AI RSS', url: 'https://developer.nvidia.com/blog/category/generative-ai/feed/', statuses: [200] },
      { name: '人人都是产品经理 RSS', url: 'https://www.woshipm.com/feed', statuses: [200] },
      { name: '极客公园 RSS', url: 'https://www.geekpark.net/rss', statuses: [200] },
      { name: 'Reddit LocalLLaMA RSS', url: 'https://www.reddit.com/r/LocalLLaMA/new/.rss', statuses: [200, 403, 429] },
      { name: 'Reddit MachineLearning RSS disabled source', url: 'https://www.reddit.com/r/MachineLearning/new/.rss', statuses: [200, 403, 429] },
      { name: 'AIHOT items API', url: 'https://aihot.news/api/v1/items?mode=all&window=7d&by=published&limit=1', statuses: [200], json: true },
    ]

    const context = await playwrightRequest.newContext({
      ignoreHTTPSErrors: true,
      extraHTTPHeaders: { 'User-Agent': 'aihot-source-playwright/0.1' },
      proxy: proxyServer(),
    })
    try {
      for (const check of checks) {
        await test.step(check.name, async () => {
          const res = await context.fetch(check.url, {
            method: check.json ? 'GET' : 'HEAD',
            timeout: 20_000,
          })
          expect(check.statuses, `${check.name} returned ${res.status()}`).toContain(res.status())
          if (check.name === 'AIHOT items API' && res.status() === 200) {
            const body = await res.json()
            expect(Array.isArray(body.items)).toBe(true)
          }
        })
      }
    } finally {
      await context.dispose()
    }
  })

  test('TC-E2E-004 真实 PostgreSQL schema 迁移：空库/已有表', async () => {
    test.skip(!process.env.AIHOT_TEST_DATABASE_URL, 'AIHOT_TEST_DATABASE_URL is required for real database migration acceptance')
    test.setTimeout(120_000)

    const result = await runCommand('go', ['test', './internal/ingest', '-count=1', '-v'], serverDir)
    await test.info().attach('go-real-db-ingest-tests.txt', {
      body: result.output,
      contentType: 'text/plain',
    })
    expect(result.output).toContain('PASS')
    expect(result.output).toContain('TestInsertRawPersistsSourceRole')
  })
})

async function runCommand(command: string, args: string[], cwd: string): Promise<{ output: string }> {
  const { stdout, stderr } = await execFileAsync(command, args, {
    cwd,
    timeout: 180_000,
    maxBuffer: 1024 * 1024 * 20,
    env: {
      ...process.env,
      NO_PROXY: mergeNoProxy(process.env.NO_PROXY),
      no_proxy: mergeNoProxy(process.env.no_proxy),
    },
  })
  return { output: `${stdout}${stderr}` }
}

function mergeNoProxy(value?: string) {
  const entries = (value ? value.split(',') : []).concat(['127.0.0.1', 'localhost'])
  return [...new Set(entries.map((entry) => entry.trim()).filter(Boolean))].join(',')
}

function proxyServer() {
  const server = process.env.HTTPS_PROXY || process.env.HTTP_PROXY || process.env.ALL_PROXY
  return server ? { server } : undefined
}

async function installSourceExpansionRoutes(page: Page) {
  const calls = { items: [] as string[], failItems: false }

  await page.route('**/api/public/version', route => fulfill(route, {
    apiVersion: 'test',
    skillVersion: 'test',
    updatedAt: '2026-09-16T00:00:00Z',
    changelogUrl: '',
    recentChanges: [],
  }))
  await page.route('**/api/tools/configs**', route => fulfillTool(route, {
    count: 0,
    enabledCount: 0,
    disabledCount: 0,
    items: [],
  }))
  await page.route('**/api/tools/items**', route => fulfillTool(route, emptyToolList()))
  await page.route(/.*\/api\/tools(\?.*)?$/, route => fulfillTool(route, emptyToolList()))
  await page.route('**/api/public/items**', route => {
    if (calls.failItems) {
      return route.fulfill({ status: 500, body: 'failed' })
    }
    const url = new URL(route.request().url())
    calls.items.push(url.search)
    const sourceKind = url.searchParams.get('source_kind') ?? 'all'
    return fulfill(route, {
      count: 1,
      hasNext: false,
      nextCursor: null,
      items: [itemForSource(sourceKind)],
    })
  })

  return calls
}

function itemForSource(sourceKind: string): PublicItem {
  switch (sourceKind) {
    case 'rss':
      return item('rss-1', 'RSS 官方资讯', 'OpenAI Blog', true, 'official')
    case 'html':
      return item('html-1', 'HTML 网页直采资讯', 'Anthropic News', true, 'official')
    case 'mp':
      return item('mp-1', '公众号语料资讯', 'WeChat MP', true, 'professional')
    case 'aihot':
      return item('aihot-1', 'AIHOT 二手补漏资讯', 'AIHOT · 二手线索', false, 'discovery')
    default:
      return item('all-1', '全部来源汇总', 'AI Cool', true, 'discovery')
  }
}

function item(id: string, title: string, source: string, selected: boolean, sourceRole: string): PublicItem {
  return {
    id,
    title,
    source,
    selected,
    source_role: sourceRole,
    url: `https://example.com/${id}`,
    permalink: `/items/${id}`,
    category: 'ai-models',
    score: selected ? 4 : undefined,
    summary: `${title} 摘要`,
  }
}

function emptyToolList() {
  return {
    count: 0,
    page: 1,
    pageSize: 50,
    offset: 0,
    take: 50,
    stats: {
      status: 'discovered',
      keywordSourceCount: 0,
      topicSourceCount: 0,
      manualSourceCount: 0,
      linkedEvaluationCount: 0,
      unlinkedEvaluationCount: 0,
      evaluatorCount: 0,
    },
    items: [],
  }
}

function fulfill(route: Route, body: unknown) {
  return route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(body),
  })
}

function fulfillTool(route: Route, data: unknown) {
  return fulfill(route, { errno: 0, errmsg: '', data })
}
