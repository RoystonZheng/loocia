import { expect, test, type APIRequestContext } from '@playwright/test'
import { execFileSync, spawnSync } from 'node:child_process'
import { chmodSync, mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import os from 'node:os'

type DailySummary = {
  date: string
  leadTitle: string
}

type DailyReport = {
  date: string
  sections: { label: string; items: { title: string; sourceName: string }[] }[]
  flashes: { title: string; sourceName: string }[]
}

type ItemList = {
  count: number
  hasNext: boolean
  nextCursor: string | null
  items: {
    id: string
    title: string
    sourceKind?: string
    category?: string
    score?: number
    selected: boolean
  }[]
}

type ToolEnvelope<T> = {
  errno: number
  errmsg: string
  data: T
}

type ToolList = {
  count: number
  stats: {
    purposeTags: string[]
    keywords: string[]
    latestUpdatedAt?: string
  }
  items: {
    id: string
    name: string
    githubFullName: string
    purposeTags: string[]
    status: string
  }[]
}

type ConfigList = {
  count: number
  enabledCount: number
  disabledCount: number
  items: {
    id: string
    name: string
    method: string
    terms: string[]
    triggerMode: string
    scheduleTime?: string
    scheduleWeekday?: number
    enabled: boolean
  }[]
}

const apiBase = process.env.AI_COOL_API_BASE ?? 'http://127.0.0.1:8991'

test.describe('AI Cool 本地真实环境验收', () => {
  test.skip(process.env.AI_COOL_LIVE_ACCEPTANCE !== '1', '设置 AI_COOL_LIVE_ACCEPTANCE=1 后才运行真实本地环境验收')

  test('ACCEPT-001 前端 5718 和后端 8991 连通', async ({ request }) => {
    const home = await request.get('/')
    expect(home.status(), '5718 首页必须能访问').toBe(200)
    expect(await home.text()).toContain('AI Cool')

    const health = await request.get(`${apiBase}/healthz`)
    expect(health.status(), '8991 /healthz 必须健康').toBe(200)
    expect(await health.json()).toEqual({ status: 'ok' })

    const version = await json<{ apiVersion: string; skillVersion: string }>(request, '/api/public/version')
    expect(version.apiVersion).toBeTruthy()
    expect(version.skillVersion).toBeTruthy()
  })

  test('ACCEPT-002 资讯、日报、热点和词云接口可用', async ({ request }) => {
    const dailies = await json<{ items: DailySummary[] }>(request, '/api/public/dailies')
    expect(dailies.items.length, '至少要有一篇日报用于验收').toBeGreaterThan(0)

    const report = await json<DailyReport>(request, `/api/public/daily/${dailies.items[0].date}`)
    const storyCount = report.sections.reduce((sum, section) => sum + section.items.length, 0)
    expect(report.date).toBe(dailies.items[0].date)
    expect(storyCount, '日报里至少应有一条资讯').toBeGreaterThan(0)

    const hot = await json<{ count: number; items: { title: string; sourceCount: number }[] }>(request, '/api/public/hot-topics')
    expect(hot.count).toBe(hot.items.length)
    if (hot.items.length > 0) {
      expect(hot.items[0].title).toBeTruthy()
      expect(hot.items[0].sourceCount).toBeGreaterThan(0)
    }

    const cloud = await json<{ terms: { term: string; count: number }[] }>(request, '/api/public/graph/cloud?window=30d')
    expect(cloud.terms.length, '30 天词云必须有数据').toBeGreaterThan(0)
    expect(cloud.terms[0].term).toBeTruthy()
    expect(cloud.terms[0].count).toBeGreaterThan(0)

    const term = await json<{ term: string; neighbors: unknown[]; items: unknown[] }>(
      request,
      `/api/public/graph/term/${encodeURIComponent(cloud.terms[0].term)}?window=30d`,
    )
    expect(term.term).toBe(cloud.terms[0].term)
    expect(Array.isArray(term.neighbors)).toBe(true)
    expect(Array.isArray(term.items)).toBe(true)
  })

  test('ACCEPT-003 全部资讯筛选链路可在页面操作', async ({ request, page }) => {
    const allItems = await json<ItemList>(request, '/api/public/items?mode=all&take=1')
    expect(Array.isArray(allItems.items)).toBe(true)
    if (allItems.items.length === 0) {
      test.info().annotations.push({
        type: 'local-data',
        description: '当前 7 天全量资讯为空，继续用公众号历史数据验证筛选 UI。',
      })
    }

    const mpItems = await json<ItemList>(request, '/api/public/items?mode=all&source_kind=mp&take=1')
    expect(mpItems.items.length, '公众号来源至少要有一条历史数据用于筛选验收').toBeGreaterThan(0)
    expect(mpItems.items[0].sourceKind).toBe('mp')

    await page.goto('/#all')
    await expect(page.getByRole('heading', { name: 'AI 动态' })).toBeVisible()
    await page.getByRole('button', { name: '公众号' }).click()
    await expect(page.getByText(mpItems.items[0].title)).toBeVisible()

    await page.getByLabel('资讯筛选').selectOption('score-3')
    await expect(page.locator('.feed-error')).toHaveCount(0)
    await expect(page.getByText('加载失败，请稍后重试。')).toHaveCount(0)

    await page.getByPlaceholder('搜索标题/摘要/正文…').fill('Agent')
    await page.getByRole('button', { name: '搜索' }).click()
    await expect(page.locator('.feed-error')).toHaveCount(0)
  })

  test('ACCEPT-004 工具列表、发现配置和运行设置读取正常', async ({ request, page }) => {
    const tools = await toolJson<ToolList>(request, '/api/tools/items?take=1')
    expect(tools.count, '工具池应有数据').toBeGreaterThan(0)
    expect(tools.items.length).toBeGreaterThan(0)
    expect(tools.stats.purposeTags.length, '工具用途标签应可用于筛选').toBeGreaterThan(0)
    expect(tools.stats.keywords.length, '配置关键词应可用于筛选').toBeGreaterThan(0)

    const configs = await toolJson<ConfigList>(request, '/api/tools/configs')
    expect(configs.count).toBe(configs.items.length)
    expect(configs.enabledCount + configs.disabledCount).toBe(configs.count)
    expect(configs.items.length, '至少要有一条发现配置').toBeGreaterThan(0)
    for (const cfg of configs.items) {
      expect(['keyword', 'topic']).toContain(cfg.method)
      expect(['manual', 'daily', 'weekly']).toContain(cfg.triggerMode)
      expect(cfg.terms.length).toBeGreaterThan(0)
      if (cfg.triggerMode === 'daily' || cfg.triggerMode === 'weekly') {
        expect(cfg.scheduleTime, `${cfg.name} 必须配置运行时间`).toMatch(/^\d{2}:\d{2}$/)
      }
      if (cfg.triggerMode === 'weekly') {
        expect(cfg.scheduleWeekday, `${cfg.name} 每周触发必须配置星期`).toBeGreaterThanOrEqual(1)
        expect(cfg.scheduleWeekday, `${cfg.name} 每周触发必须配置星期`).toBeLessThanOrEqual(7)
      }
    }

    const settings = await toolJson<{ settings: { githubTokens: { masked: string; last4: string }[]; githubTokenStrategy: string } }>(
      request,
      '/api/tools/settings',
    )
    expect(['round_robin', 'fixed', 'failover']).toContain(settings.settings.githubTokenStrategy)
    for (const token of settings.settings.githubTokens) {
      expect(token.masked).toContain('****')
      expect(token.masked).not.toBe(token.last4)
    }

    await page.goto('/#tools-discovered')
    await expect(page.getByRole('heading', { name: '工具百宝箱' })).toBeVisible()
    await expect(page.getByText(tools.items[0].name).first()).toBeVisible()
    await page.getByLabel('用途', { exact: true }).selectOption(tools.stats.purposeTags[0])
    await expect(page.locator('.tools-error')).toHaveCount(0)
  })

  test('ACCEPT-005 关键必填项在前端阻断，不误写本地库', async ({ page }) => {
    await page.goto('/#tools-configs')
    await page.getByRole('button', { name: /新增配置/ }).click()
    const configDialog = page.getByRole('dialog', { name: '新增发现配置' })
    await configDialog.getByLabel('配置名称').fill('本地自动化必填项校验')
    await configDialog.getByLabel('检索词').fill('local acceptance smoke')
    await configDialog.getByRole('button', { name: '创建配置' }).click()
    await expect(configDialog.getByText('请先填写操作人')).toBeVisible()
    await page.keyboard.press('Escape')

    await page.goto('/#tools-discovered')
    await page.getByRole('button', { name: /手动添加/ }).click()
    const manualDialog = page.getByRole('dialog', { name: '手动添加 GitHub 工具' })
    await manualDialog.getByRole('button', { name: '预览' }).click()
    await expect(manualDialog.getByText('请填写 GitHub 仓库地址')).toBeVisible()
    await manualDialog.getByLabel('GitHub 仓库地址').fill('不是一个仓库地址')
    await manualDialog.getByRole('button', { name: '预览' }).click()
    await expect(manualDialog.getByText(/请输入有效的 GitHub 仓库地址|invalid github/i)).toBeVisible()
  })

  test('ACCEPT-006 本地自动触发器已安装并指向本地库', async () => {
    test.skip(os.platform() !== 'darwin', '本地触发器验收只适用于 macOS LaunchAgent/cron 环境')

    const cron = execFileSync('crontab', ['-l'], { encoding: 'utf8' })
    expect(cron).toContain('local-aihot-pulse-trigger.sh')
    expect(cron).toContain('local-aihot-discovertools-trigger.sh scheduled')
    expect(cron).toContain('local-aihot-discovertools-trigger.sh snapshot')

    // gendaily/hotpass are derived jobs of a successful pulse, so they do not
    // need independent cron entries and cannot overlap a still-running pulse.
    const scriptsDir = path.resolve(process.cwd(), '../server/scripts')
    const pulseTrigger = readFileSync(path.join(scriptsDir, 'local-aihot-pulse-trigger.sh'), 'utf8')
    expect(pulseTrigger).toContain('local-aihot-unit-trigger.sh')
    expect(pulseTrigger).toContain('gendaily')
    expect(pulseTrigger).toContain('hotpass')

    const uid = typeof process.getuid === 'function' ? process.getuid() : ''
    const launch = execFileSync('launchctl', ['print', `gui/${uid}/com.aihot.local-server`], { encoding: 'utf8' })
    expect(launch).toContain('state = running')
    expect(launch).toContain('127.0.0.1:8991')
    expect(launch).toContain('aihot_local_prod_clone')
  })

  test('ACCEPT-007 本地触发器不依赖生产 SSH', async () => {
    const tempRoot = mkdtempSync(path.join(os.tmpdir(), 'aihot-local-trigger-'))
    const fakeBinDir = path.join(tempRoot, 'bin')
    const fakeEnvFile = path.join(tempRoot, 'missing.env')
    mkdirSync(fakeBinDir)
    const fakeDiscoverTools = path.join(fakeBinDir, 'discovertools')
    writeFileSync(
      fakeDiscoverTools,
      '#!/usr/bin/env bash\nprintf "local-db=%s args=%s\\n" "$AI_TOOL_DATABASE_URL" "$*"\n',
      'utf8',
    )
    chmodSync(fakeDiscoverTools, 0o700)

    try {
      const script = path.resolve(process.cwd(), '../server/scripts/local-aihot-cron-safe.sh')
      const result = spawnSync(
        'bash',
        [script, 'discovertools', 'scheduled', '-actor', 'local-acceptance'],
        {
          encoding: 'utf8',
          env: {
            ...process.env,
            AIHOT_LOCAL_BIN_DIR: fakeBinDir,
            AIHOT_LOCAL_ENV_FILE: fakeEnvFile,
            AIHOT_GO_BIN: '/usr/bin/true',
            AIHOT_DATABASE_URL: 'postgres://didi@localhost:5432/aihot_local_prod_clone?sslmode=disable',
            AI_TOOL_DATABASE_URL: 'postgres://didi@localhost:5432/aihot_local_prod_clone?sslmode=disable',
          },
        },
      )
      expect(result.status, result.stderr).toBe(0)
      expect(result.stdout).toContain('local-db=postgres://didi@localhost:5432/aihot_local_prod_clone')
      expect(result.stdout).toContain('scheduled -actor local-acceptance')
      expect(readFileSync(script, 'utf8')).not.toContain('10.190.12.242')
    } finally {
      rmSync(tempRoot, { recursive: true, force: true })
    }
  })

  test('ACCEPT-008 无 LLM 凭据时本地 pulse 仍执行原始采集模式', async () => {
    const tempRoot = mkdtempSync(path.join(os.tmpdir(), 'aihot-local-pulse-'))
    const fakeBinDir = path.join(tempRoot, 'bin')
    const fakeEnvFile = path.join(tempRoot, 'missing.env')
    mkdirSync(fakeBinDir)
    const fakePulse = path.join(fakeBinDir, 'pulse')
    writeFileSync(
      fakePulse,
      '#!/usr/bin/env bash\nprintf "ingest-only=%s args=%s\\n" "$AIHOT_PULSE_INGEST_ONLY" "$*"\n',
      'utf8',
    )
    chmodSync(fakePulse, 0o700)

    try {
      const script = path.resolve(process.cwd(), '../server/scripts/local-aihot-cron-safe.sh')
      const result = spawnSync('bash', [script, 'pulse'], {
        encoding: 'utf8',
        env: {
          ...process.env,
          AIHOT_LOCAL_BIN_DIR: fakeBinDir,
          AIHOT_LOCAL_ENV_FILE: fakeEnvFile,
          AIHOT_GO_BIN: '/usr/bin/true',
          AIHOT_DATABASE_URL: 'postgres://didi@localhost:5432/aihot_local_prod_clone?sslmode=disable',
          AI_TOOL_DATABASE_URL: 'postgres://didi@localhost:5432/aihot_local_prod_clone?sslmode=disable',
        },
      })
      expect(result.status, result.stderr).toBe(0)
      expect(result.stderr).toContain('ingest-only mode')
      expect(result.stdout).toContain('ingest-only=1')
    } finally {
      rmSync(tempRoot, { recursive: true, force: true })
    }
  })
})

async function json<T>(request: APIRequestContext, path: string): Promise<T> {
  const response = await request.get(path)
  expect(response.ok(), `${path} should return 2xx, got ${response.status()}`).toBe(true)
  return await response.json() as T
}

async function toolJson<T>(request: APIRequestContext, path: string): Promise<T> {
  const envelope = await json<ToolEnvelope<T>>(request, path)
  expect(envelope.errno, `${path} errno`).toBe(0)
  expect(envelope.errmsg, `${path} errmsg`).toBe('ok')
  return envelope.data
}
