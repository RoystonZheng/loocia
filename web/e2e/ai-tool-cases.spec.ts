import { expect, test, type Page, type Request, type Route } from '@playwright/test'

type DiscoveryMethod = 'keyword' | 'topic'
type TriggerMode = 'manual' | 'weekly'
type ToolStatus = 'discovered' | 'evaluating' | 'included' | 'excluded'
type ToolSort = 'latest' | 'stars' | 'stars7d'
type ToolSourceType = 'keyword' | 'topic' | 'manual'
type RunStatus = 'running' | 'paused' | 'succeeded' | 'partial' | 'failed'

type DiscoveryConfig = {
  id: string
  name: string
  method: DiscoveryMethod
  terms: string[]
  triggerMode: TriggerMode
  enabled: boolean
  lastSuccessAt?: string
  lastRunAt?: string
  lastRunStatus?: RunStatus
  lastResultCount: number
  lastPagesScanned: number
  lastPauseRequested: boolean
  lastFailureReason?: string
  createdBy: string
  updatedBy: string
  createdAt: string
  updatedAt: string
}

type ToolSource = {
  sourceType: ToolSourceType
  term: string
  configId?: string
  lastSeenAt: string
  hitCount: number
}

type ToolEvaluation = {
  id: string
  evaluator: string
  operator: string
  cooperUrl?: string
  startedAt: string
  completedAt?: string
  result?: 'included' | 'excluded'
  finalSummary?: string
  notIncludedReason?: string
}

type ToolItem = {
  id: string
  name: string
  githubNodeId: string
  githubOwner: string
  githubRepo: string
  githubFullName: string
  githubUrl: string
  description?: string
  temporarySummary?: string
  finalSummary?: string
  currentSummary?: string
  currentSummarySource: 'final' | 'temporary' | 'github_description' | 'none'
  stars: number
  forks: number
  openIssues: number
  stars7d?: number
  topics: string[]
  licenseSpdx?: string
  defaultBranch?: string
  pushedAt?: string
  status: ToolStatus
  firstDiscoveredAt: string
  lastDiscoveredAt: string
  includedAt?: string
  excludedAt?: string
  excludedStage?: 'discovered' | 'evaluating'
  excludedReason?: string
  statusVersion: number
  summaryKey: string
  sources: ToolSource[]
  evaluation?: ToolEvaluation
  createdAt: string
  updatedAt: string
}

type MockState = {
  configs: DiscoveryConfig[]
  tools: ToolItem[]
  calls: { path: string; body?: unknown }[]
}

const now = '2026-09-11T02:00:00Z'
const cooperUrl = 'https://cooper.didichuxing.com/didocs/2209600482571'

test.describe('AI Tool 开发用例', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      window.confirm = () => true
    })
  })

  test('TC-001~TC-009 发现配置：新增、检索、筛选、编辑、停用、删除', async ({ page }) => {
    const state = await installMockRoutes(page, createMockState())

    await test.step('TC-001 打开发现配置页并展示统计', async () => {
      await page.goto('/#tools-configs')
      await expect(page.getByRole('heading', { name: '发现配置' })).toBeVisible()
      await expect(page.getByText('4条配置')).toBeVisible()
      await expect(page.getByText('3已启用')).toBeVisible()
      await expect(page.getByText('1已停用')).toBeVisible()
    })

    await test.step('TC-002 发现方式和触发方式使用下拉筛选', async () => {
      await page.getByLabel('发现方式').selectOption('topic')
      await page.getByLabel('触发方式').selectOption('weekly')
      await expect(page.getByText('上线验证-Topic')).toBeVisible()
      await expect(page.getByText('流程图工具')).toBeHidden()
    })

    await test.step('TC-003 按配置名或检索词搜索', async () => {
      await page.getByLabel('发现方式').selectOption('all')
      await page.getByLabel('触发方式').selectOption('all')
      await runSearch(page, '搜索配置或检索词', 'browser')
      await expect(page.getByText('浏览器操作工具')).toBeVisible()
      await expect(page.getByText('上线验证-Topic')).toBeHidden()
    })

    await test.step('TC-004 新增配置必须写操作人，成功后保存 actor', async () => {
      await page.getByRole('button', { name: /新增配置/ }).click()
      const dialog = page.getByRole('dialog', { name: '新增发现配置' })
      await dialog.getByLabel('配置名称').fill('Playwright 配置')
      await dialog.getByLabel('检索词').fill('playwright agent')
      await dialog.getByRole('button', { name: '创建配置' }).click()
      await expect(page.getByText('请先填写操作人')).toBeVisible()
      await dialog.getByLabel('操作人').fill('郑睿涛')
      await dialog.getByRole('button', { name: '创建配置' }).click()
      await expect(page.getByText('配置已创建')).toBeVisible()
      expect(lastCall(state, '/api/tools/configs')?.body).toMatchObject({ actor: '郑睿涛' })
    })

    await test.step('TC-005 编辑配置可以复用已有操作人', async () => {
      await runSearch(page, '搜索配置或检索词', 'Playwright')
      await configCard(page, 'Playwright 配置').getByRole('button', { name: '编辑' }).click()
      const dialog = page.getByRole('dialog', { name: '编辑发现配置' })
      await dialog.getByLabel('配置名称').fill('Playwright 配置-已编辑')
      await dialog.getByRole('button', { name: '保存配置' }).click()
      await expect(page.getByText('配置已更新')).toBeVisible()
      await expect(page.getByText('Playwright 配置-已编辑')).toBeVisible()
    })

    await test.step('TC-006 停用和启用配置走 actor 链路', async () => {
      await configCard(page, 'Playwright 配置-已编辑').getByRole('button', { name: '停用' }).click()
      await expect(page.getByText('配置已停用')).toBeVisible()
      expect(lastCall(state, '/api/tools/configs/enable')?.body).toMatchObject({ enabled: false, actor: '郑睿涛' })
      await configCard(page, 'Playwright 配置-已编辑').getByRole('button', { name: '启用' }).click()
      await expect(page.getByText('配置已启用')).toBeVisible()
    })

    await test.step('TC-007 删除配置只删除配置，保留历史工具', async () => {
      await configCard(page, 'Playwright 配置-已编辑').getByRole('button', { name: '删除' }).click()
      await expect(page.getByText('配置已删除')).toBeVisible()
      expect(lastCall(state, '/api/tools/configs/delete')?.body).toMatchObject({ actor: '郑睿涛' })
      await expect(page.getByText('Playwright 配置-已编辑')).toBeHidden()
    })

    await test.step('TC-008 清空搜索后恢复配置列表', async () => {
      await runSearch(page, '搜索配置或检索词', '')
      await expect(page.getByText('流程图工具')).toBeVisible()
      await expect(page.getByText('上线验证-Topic')).toBeVisible()
    })

    await test.step('TC-009 配置卡片展示执行、编辑、停用和删除操作', async () => {
      const card = configCard(page, '流程图工具')
      await expect(card.getByRole('button', { name: '执行' })).toBeVisible()
      await expect(card.getByRole('button', { name: '编辑' })).toBeVisible()
      await expect(card.getByRole('button', { name: '停用' })).toBeVisible()
      await expect(card.getByRole('button', { name: '删除' })).toBeVisible()
    })
  })

  test('TC-010~TC-013 发现配置运行：刷新不中断，按钮支持暂停和继续', async ({ page }) => {
    const state = await installMockRoutes(page, createMockState())

    await page.goto('/#tools-configs')
    const card = configCard(page, '浏览器操作工具')

    await test.step('TC-010 点击执行后进入运行态', async () => {
      await card.getByRole('button', { name: '执行' }).click()
      await expect(page.getByText('已开始执行，刷新页面不会中断')).toBeVisible()
      await expect(card.getByRole('button', { name: '暂停' })).toBeVisible()
      expect(state.calls.filter((call) => call.path === '/api/tools/configs/run')).toHaveLength(1)
    })

    await test.step('TC-011 刷新页面后仍显示运行态和进度', async () => {
      await page.reload()
      await expect(page.getByRole('heading', { name: '发现配置' })).toBeVisible()
      await expect(configCard(page, '浏览器操作工具').getByRole('button', { name: '暂停' })).toBeVisible()
      await expect(configCard(page, '浏览器操作工具').getByText(/运行中/)).toBeVisible()
    })

    await test.step('TC-012 再次点击运行按钮会暂停，不重复发起 run', async () => {
      await configCard(page, '浏览器操作工具').getByRole('button', { name: '暂停' }).click()
      await expect(page.getByText(/配置已暂停|已请求暂停/)).toBeVisible()
      await expect(configCard(page, '浏览器操作工具').getByRole('button', { name: '继续' })).toBeVisible()
      expect(state.calls.filter((call) => call.path === '/api/tools/configs/run')).toHaveLength(1)
      expect(state.calls.filter((call) => call.path === '/api/tools/configs/pause')).toHaveLength(1)
    })

    await test.step('TC-013 暂停后可以继续执行', async () => {
      await configCard(page, '浏览器操作工具').getByRole('button', { name: '继续' }).click()
      await expect(page.getByText('已继续执行')).toBeVisible()
      await expect(configCard(page, '浏览器操作工具').getByRole('button', { name: '暂停' })).toBeVisible()
      expect(state.calls.filter((call) => call.path === '/api/tools/configs/resume')).toHaveLength(1)
    })
  })

  test('TC-014~TC-024 已发现工具：排序、筛选、中文介绍、手动添加、单个测评/删除、跨页批量', async ({ page }) => {
    const state = await installMockRoutes(page, createMockState({ discoveredCount: 105 }))

    await test.step('TC-014 打开已发现工具并展示分页', async () => {
      await page.goto('/#tools-discovered')
      await expect(page.getByRole('heading', { name: '已发现工具' })).toBeVisible()
      await expect(page.getByText(/第 1-50 条 \/ 共 105 条/)).toBeVisible()
      await expect(page.getByRole('button', { name: '下一页' })).toBeEnabled()
    })

    await test.step('TC-015 三种排序调用相同集合，仅改变顺序', async () => {
      await page.getByRole('button', { name: /当前 Stars/ }).click()
      await expect(firstToolName(page)).toContainText('DeepResearch AI')
      await page.getByRole('button', { name: /7 日 Star 增长/ }).click()
      await expect(firstToolName(page)).toContainText('增长爆发工具')
      await page.getByRole('button', { name: /最新发现/ }).click()
      await expect(page.getByText(/第 1-50 条 \/ 共 105 条/)).toBeVisible()
    })

    await test.step('TC-016 来源筛选恢复为下拉菜单', async () => {
      await sourceFilter(page).selectOption('topic')
      await expect(toolName(page, 'Topic 命中工具')).toBeVisible()
      await expect(toolName(page, '手动添加工具')).toBeHidden()
      await sourceFilter(page).selectOption('manual')
      await expect(toolName(page, '手动添加工具')).toBeVisible()
      await expect(toolName(page, 'Topic 命中工具')).toBeHidden()
      await sourceFilter(page).selectOption('all')
    })

    await test.step('TC-017 搜索工具名称、仓库和来源关键词', async () => {
      await runSearch(page, '搜索工具、仓库或发现来源', 'DeepResearch')
      await expect(page.getByText('DeepResearch AI')).toBeVisible()
      await expect(page.getByText(/1 个结果/)).toBeVisible()
    })

    await test.step('TC-018 英文介绍自动走 url key 中文接口，悬浮展示完整介绍', async () => {
      await runSearch(page, '搜索工具、仓库或发现来源', 'DeepResearch')
      await expect(page.getByText('用于深度研究、资料整理和长文生成的中文说明。')).toBeVisible()
      await page.getByText('用于深度研究、资料整理和长文生成的中文说明。').hover()
      await expect(page.locator('.tools-floating-tooltip')).toContainText('用于深度研究、资料整理和长文生成的中文说明。')
      expect(state.calls.some((call) => call.path === '/api/tools/summaries/url-key')).toBe(true)
    })

    await test.step('TC-019 真实 GitHub 工具名称可点击跳转到原仓库', async () => {
      await expect(page.getByRole('link', { name: /打开 GitHub：Aryan-Pardeshi\/DeepResearch_AI/ })).toHaveAttribute('href', 'https://github.com/Aryan-Pardeshi/DeepResearch_AI')
    })

    await test.step('TC-020 手动添加公开 GitHub 仓库', async () => {
      await page.getByRole('button', { name: /手动添加/ }).click()
      const dialog = page.getByRole('dialog', { name: '手动添加 GitHub 工具' })
      await dialog.getByLabel('GitHub 仓库地址').fill('https://github.com/anthropics/skills')
      await dialog.getByRole('button', { name: '预览' }).click()
      await expect(dialog.getByText('Anthropic Skills')).toBeVisible()
      await dialog.getByRole('button', { name: '加入已发现' }).click()
      await expect(dialog.getByText('请填写操作人')).toBeVisible()
      await dialog.getByLabel('操作人').fill('郑睿涛')
      await dialog.getByRole('button', { name: '加入已发现' }).click()
      await expect(page.getByText('仓库已加入已发现列表')).toBeVisible()
    })

    await test.step('TC-021 单个开始测评要求测评人和操作人', async () => {
      await runSearch(page, '搜索工具、仓库或发现来源', 'DeepResearch')
      await page.getByRole('button', { name: '开始测评' }).click()
      const dialog = page.getByRole('dialog', { name: /开始测评/ })
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(dialog.getByText('请填写测评人')).toBeVisible()
      await dialog.getByLabel('测评人').fill('李明')
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(dialog.getByText('请填写操作人')).toBeVisible()
      await dialog.getByLabel('操作人').fill('郑睿涛')
      await dialog.getByLabel('Cooper 链接').fill(cooperUrl)
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(page.getByText('已开始测评')).toBeVisible()
    })

    await test.step('TC-022 单个不处理必须写原因和操作人', async () => {
      await runSearch(page, '搜索工具、仓库或发现来源', 'Manual')
      await toolRow(page, '手动添加工具').getByRole('button', { name: '不处理' }).click()
      const dialog = page.getByRole('dialog', { name: /不处理/ })
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(dialog.getByText('请填写操作人')).toBeVisible()
      await dialog.getByLabel('操作人').fill('郑睿涛')
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(dialog.getByText('请填写原因')).toBeVisible()
      await dialog.getByLabel('不处理原因').fill('与团队场景不匹配')
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(page.getByText('已标记不处理')).toBeVisible()
    })

    await test.step('TC-023 跨页选择后批量测试分别保留所选工具', async () => {
      await runSearch(page, '搜索工具、仓库或发现来源', '')
      await page.getByRole('button', { name: /最新发现/ }).click()
      await page.getByLabel('选择 batch-owner-0/repo-0').check()
      await page.getByRole('button', { name: '下一页' }).click()
      await page.getByLabel('选择 batch-owner-50/repo-50').check()
      await expect(page.getByText(/已跨页选择 2 条/)).toBeVisible()
      await page.getByRole('button', { name: '批量测试' }).click()
      const dialog = page.getByRole('dialog', { name: /批量开始测评/ })
      await dialog.getByLabel('测评人').fill('李明')
      await dialog.getByLabel('操作人').fill('郑睿涛')
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(page.getByText('批量测评完成：2 个工具')).toBeVisible()
    })

    await test.step('TC-024 批量删除必须写删除原因', async () => {
      await resetToolPage(page, '/#tools-discovered')
      await page.getByLabel('每页数量').selectOption('200')
      await page.getByLabel('选择 batch-owner-1/repo-1').check()
      await page.getByLabel('选择 batch-owner-2/repo-2').check()
      await page.getByRole('button', { name: '批量删除' }).click()
      const dialog = page.getByRole('dialog', { name: /批量删除已发现工具/ })
      await dialog.getByLabel('操作人').fill('郑睿涛')
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(dialog.getByText('请填写删除原因')).toBeVisible()
      await dialog.getByLabel('不处理原因').fill('批量清理测试数据')
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(page.getByText('批量删除完成：2 个工具')).toBeVisible()
    })
  })

  test('TC-025~TC-033 测评中：检索、来源筛选、关联文档、纳入、不纳入和批量操作', async ({ page }) => {
    await installMockRoutes(page, createMockState())

    await test.step('TC-025 打开测评中并展示文档关联统计', async () => {
      await page.goto('/#tools-evaluating')
      await expect(page.getByRole('heading', { name: '测评中' })).toBeVisible()
      await expect(page.getByText('7测评中')).toBeVisible()
      await expect(page.getByText('5已关联文档')).toBeVisible()
      await expect(page.getByText('2待关联文档')).toBeVisible()
    })

    await test.step('TC-026 测评中支持来源下拉筛选和搜索测评人', async () => {
      await sourceFilter(page).selectOption('topic')
      await expect(toolName(page, 'Superpowers')).toBeVisible()
      await expect(toolName(page, 'Browser Use')).toBeHidden()
      await sourceFilter(page).selectOption('all')
      await runSearch(page, '搜索工具或测评人', '周恺')
      await expect(toolName(page, 'Browser Use')).toBeVisible()
      await expect(toolName(page, 'Superpowers')).toBeHidden()
    })

    await test.step('TC-027 关联 Cooper 文档要求操作人和链接', async () => {
      await runSearch(page, '搜索工具或测评人', 'Need Cooper')
      await page.getByRole('button', { name: '关联文档' }).click()
      const dialog = page.getByRole('dialog', { name: /关联 Cooper 文档/ })
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(dialog.getByText('请填写操作人')).toBeVisible()
      await dialog.getByLabel('操作人').fill('郑睿涛')
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(dialog.getByText('请填写 Cooper 链接')).toBeVisible()
      await dialog.getByLabel('Cooper 链接').fill(cooperUrl)
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(page.getByText('Cooper 链接已更新')).toBeVisible()
    })

    await test.step('TC-028 单个纳入团队工具要求操作人', async () => {
      await runSearch(page, '搜索工具或测评人', 'Browser Use')
      await page.getByRole('button', { name: '✓ 纳入' }).click()
      const dialog = page.getByRole('dialog', { name: /纳入团队工具/ })
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(dialog.getByText('请填写操作人')).toBeVisible()
      await dialog.getByLabel('操作人').fill('郑睿涛')
      await dialog.getByLabel('团队使用说明').fill('团队用于验证浏览器自动化任务，覆盖网页操作和回归场景。')
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(page.getByText('已纳入团队工具')).toBeVisible()
    })

    await test.step('TC-029 单个不纳入要求原因', async () => {
      await page.goto('/#tools-evaluating')
      await runSearch(page, '搜索工具或测评人', 'Superpowers')
      await page.getByRole('button', { name: '不纳入' }).click()
      const dialog = page.getByRole('dialog', { name: /不纳入/ })
      await dialog.getByLabel('操作人').fill('郑睿涛')
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(dialog.getByText('请填写原因')).toBeVisible()
      await dialog.getByLabel('不纳入原因').fill('测评结论不满足团队使用标准')
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(page.getByText('已完成为不纳入')).toBeVisible()
    })

    await test.step('TC-030 批量纳入支持分别填写 Cooper 文档和说明', async () => {
      await resetToolPage(page, '/#tools-evaluating')
      await page.getByLabel('选择 batch-include/alpha').check()
      await page.getByLabel('选择 batch-include/beta').check()
      await page.getByRole('button', { name: '批量纳入' }).click()
      const dialog = page.getByRole('dialog', { name: /批量纳入团队工具/ })
      await dialog.getByLabel('操作人').fill('郑睿涛')
      await dialog.getByPlaceholder('https://cooper.didichuxing.com/didocs/...').last().fill(cooperUrl)
      const summaries = dialog.getByLabel('团队使用说明')
      await summaries.nth(0).fill('团队用于验证浏览器自动化任务，覆盖网页操作和回归场景。')
      await summaries.nth(1).fill('团队用于补齐研发流程中的上下文检索，适合内部工具链验证。')
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(page.getByText('批量纳入完成：2 个工具')).toBeVisible()
    })

    await test.step('TC-031 批量不纳入支持分别填写原因', async () => {
      await resetToolPage(page, '/#tools-evaluating')
      await page.getByLabel('选择 batch-eval/alpha').check()
      await page.getByLabel('选择 batch-eval/beta').check()
      await page.getByRole('button', { name: '批量不纳入' }).click()
      const dialog = page.getByRole('dialog', { name: /批量不纳入/ })
      await dialog.getByLabel('操作人').fill('郑睿涛')
      const reasons = dialog.getByLabel('不纳入原因')
      await reasons.nth(0).fill('稳定性不足')
      await reasons.nth(1).fill('场景不匹配')
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(page.getByText('批量不纳入完成：2 个工具')).toBeVisible()
    })

    await test.step('TC-032 已关联 Cooper 文档可点击查看', async () => {
      await resetToolPage(page, '/#tools-evaluating')
      await runSearch(page, '搜索工具或测评人', 'Need Cooper')
      await expect(page.getByRole('link', { name: /查看测评文档/ })).toHaveAttribute('href', cooperUrl)
    })

    await test.step('TC-033 批量操作结束后支持取消选择', async () => {
      await resetToolPage(page, '/#tools-evaluating')
      await page.getByLabel('选择 internal/need-cooper').check()
      await expect(page.getByText('1 已选')).toBeVisible()
      await page.getByRole('button', { name: '取消选择' }).click()
      await expect(page.getByText('1 已选')).toBeHidden()
    })
  })

  test('TC-034~TC-042 团队工具：检索、排序、手动导入、编辑、删除和批量维护', async ({ page }) => {
    await installMockRoutes(page, createMockState({ includedCount: 55 }))

    await test.step('TC-034 打开团队工具并展示分页', async () => {
      await page.goto('/#tools-team')
      await expect(page.getByRole('heading', { name: '团队工具' })).toBeVisible()
      await expect(page.getByText(/第 1-50 条 \/ 共 55 条/)).toBeVisible()
    })

    await test.step('TC-035 团队工具支持来源筛选、排序和名称搜索', async () => {
      await sourceFilter(page).selectOption('keyword')
      await expect(toolName(page, 'Claude Code')).toBeVisible()
      await expect(toolName(page, 'Context7')).toBeHidden()
      await sourceFilter(page).selectOption('all')
      await page.getByRole('button', { name: /当前 Stars/ }).click()
      await expect(firstToolName(page)).toContainText('Team Repo 51')
      await runSearch(page, '搜索团队工具', 'Playwright MCP')
      await expect(page.getByText('Playwright MCP')).toBeVisible()
    })

    await test.step('TC-036 手动导入团队工具支持可选文档和说明', async () => {
      await resetToolPage(page, '/#tools-team')
      await page.getByRole('button', { name: /手动导入/ }).click()
      const dialog = page.getByRole('dialog', { name: '手动导入团队工具' })
      await dialog.getByLabel('GitHub 仓库地址').fill('https://github.com/anthropics/claude-code')
      await dialog.getByRole('button', { name: '预览' }).click()
      await expect(dialog.getByText('Claude Code')).toBeVisible()
      await dialog.getByRole('button', { name: '加入团队工具' }).click()
      await expect(dialog.getByText('请填写操作人')).toBeVisible()
      await dialog.getByLabel('操作人').fill('郑睿涛')
      await dialog.getByLabel('Cooper 文档').fill(cooperUrl)
      await dialog.getByLabel('团队使用说明').fill('团队用于复杂代码任务首选，覆盖需求实现、调试和代码审查。')
      await dialog.getByRole('button', { name: '加入团队工具' }).click()
      await expect(page.getByText(/工具已加入团队工具|团队工具已更新/)).toBeVisible()
    })

    await test.step('TC-037 编辑团队工具要求操作人并保存说明', async () => {
      await runSearch(page, '搜索团队工具', 'Context7')
      await page.getByRole('button', { name: '编辑' }).click()
      const dialog = page.getByRole('dialog', { name: /编辑团队工具/ })
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(dialog.getByText('请填写操作人')).toBeVisible()
      await dialog.getByLabel('操作人').fill('郑睿涛')
      await dialog.getByLabel('团队使用说明').fill('团队用于补全最新官方文档上下文，减少过期 API 用法。')
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(page.getByText('团队工具已更新')).toBeVisible()
    })

    await test.step('TC-038 删除团队工具必须写删除原因且保留记录', async () => {
      await page.goto('/#tools-team')
      await runSearch(page, '搜索团队工具', 'Playwright MCP')
      await page.getByRole('button', { name: '删除' }).click()
      const dialog = page.getByRole('dialog', { name: /删除团队工具/ })
      await dialog.getByLabel('操作人').fill('郑睿涛')
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(dialog.getByText('请填写原因')).toBeVisible()
      await dialog.getByLabel('删除原因').fill('替换为内部统一方案')
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(page.getByText('团队工具已删除')).toBeVisible()
    })

    await test.step('TC-039 团队工具跨页批量修改', async () => {
      await resetToolPage(page, '/#tools-team')
      await page.getByRole('button', { name: /最新发现/ }).click()
      await page.getByLabel('选择 team-owner-0/repo-0').check()
      await page.getByRole('button', { name: '下一页' }).click()
      await page.getByLabel('选择 team-owner-50/repo-50').check()
      await expect(page.getByText(/已跨页选择 2 条/)).toBeVisible()
      await page.getByRole('button', { name: '批量修改' }).click()
      const dialog = page.getByRole('dialog', { name: /批量修改团队工具/ })
      await dialog.getByLabel('操作人').fill('郑睿涛')
      const summaries = dialog.getByLabel('团队使用说明')
      await summaries.nth(0).fill('团队工具批量修改验证说明一，覆盖跨页选择后的保存能力。')
      await summaries.nth(1).fill('团队工具批量修改验证说明二，覆盖跨页选择后的保存能力。')
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(page.getByText('批量修改完成：2 个工具')).toBeVisible()
    })

    await test.step('TC-040 团队工具批量删除必须写删除原因', async () => {
      await resetToolPage(page, '/#tools-team')
      await page.getByLabel('选择 anthropics/claude-code').check()
      await page.getByLabel('选择 upstash/context7').check()
      await page.getByRole('button', { name: '批量删除' }).click()
      const dialog = page.getByRole('dialog', { name: /批量删除团队工具/ })
      await dialog.getByLabel('操作人').fill('郑睿涛')
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(dialog.getByText('请填写删除原因')).toBeVisible()
      await dialog.getByLabel('删除原因').fill('批量清理下线工具')
      await dialog.getByRole('button', { name: '提交' }).click()
      await expect(page.getByText('批量删除完成：2 个工具')).toBeVisible()
    })

    await test.step('TC-041 团队工具保留 GitHub 和 Cooper 链接', async () => {
      await resetToolPage(page, '/#tools-team')
      const row = toolRow(page, 'Team Repo 0')
      await expect(row.locator('.tools-link-stack a', { hasText: 'GitHub' })).toHaveAttribute('href', 'https://github.com/team-owner-0/repo-0')
      await expect(row.locator('.tools-link-stack a', { hasText: 'Cooper 测评' })).toHaveAttribute('href', cooperUrl)
    })

    await test.step('TC-042 团队工具批量删除后清空选择状态', async () => {
      await page.getByLabel('选择 team-owner-0/repo-0').check()
      await expect(page.getByText('1 已选')).toBeVisible()
      await page.getByRole('button', { name: '取消选择' }).click()
      await expect(page.getByText('1 已选')).toBeHidden()
    })
  })
})

function createMockState(options: { discoveredCount?: number; includedCount?: number } = {}): MockState {
  const discoveredBase = [
    makeTool('disc-deep', 'Aryan-Pardeshi/DeepResearch_AI', {
      name: 'DeepResearch AI',
      description: 'AI research workspace with deep search and long report generation.',
      currentSummary: '',
      currentSummarySource: 'github_description',
      stars: 1800,
      stars7d: 88,
      firstDiscoveredAt: '2026-09-11T01:00:00Z',
      lastDiscoveredAt: '2026-09-11T01:00:00Z',
      pushedAt: '2026-09-11T00:00:00Z',
      sources: [{ sourceType: 'keyword', term: 'DeepResearch', configId: 'cfg-keyword', lastSeenAt: now, hitCount: 1 }],
    }),
    makeTool('disc-growth', 'growth-labs/surge-agent', {
      name: '增长爆发工具',
      currentSummary: '用于验证七日 Star 增长排序的工具。',
      stars: 900,
      stars7d: 1200,
      firstDiscoveredAt: '2026-09-10T01:00:00Z',
      sources: [{ sourceType: 'keyword', term: 'ai agent', configId: 'cfg-keyword', lastSeenAt: now, hitCount: 1 }],
    }),
    makeTool('disc-topic', 'topic-labs/agent-skills', {
      name: 'Topic 命中工具',
      currentSummary: '通过 Topic 发现的工具。',
      stars: 600,
      stars7d: 30,
      sources: [{ sourceType: 'topic', term: 'ai-agents', configId: 'cfg-topic', lastSeenAt: now, hitCount: 1 }],
    }),
    makeTool('disc-manual', 'manual-labs/manual-agent', {
      name: '手动添加工具',
      currentSummary: '通过手动添加进入已发现列表。',
      stars: 300,
      stars7d: 10,
      sources: [{ sourceType: 'manual', term: 'manual-labs/manual-agent', lastSeenAt: now, hitCount: 1 }],
    }),
  ]
  const discoveredCount = Math.max(options.discoveredCount ?? discoveredBase.length, discoveredBase.length)
  const generatedDiscovered = Array.from({ length: discoveredCount - discoveredBase.length }, (_, index) => makeTool(`disc-batch-${index}`, `batch-owner-${index}/repo-${index}`, {
    name: `Repo ${index}`,
    currentSummary: `批量和分页验证工具 ${index}`,
    stars: 1000 + index,
    stars7d: index % 9,
    firstDiscoveredAt: `2026-09-${String(9 - (index % 3)).padStart(2, '0')}T01:00:00Z`,
    sources: [{ sourceType: 'keyword', term: 'batch', configId: 'cfg-keyword', lastSeenAt: now, hitCount: 1 }],
  }))

  const evaluating = [
    makeTool('eval-browser-use', 'obrowser/browser-use', {
      name: 'Browser Use',
      status: 'evaluating',
      currentSummary: '让 AI Agent 通过浏览器完成网页操作任务。',
      sources: [{ sourceType: 'keyword', term: 'browser agent', configId: 'cfg-keyword', lastSeenAt: now, hitCount: 1 }],
      evaluation: { id: 'ev-browser-use', evaluator: '周恺', operator: '郑睿涛', cooperUrl, startedAt: '2026-09-10T03:00:00Z' },
    }),
    makeTool('eval-superpowers', 'obra/superpowers', {
      name: 'Superpowers',
      status: 'evaluating',
      currentSummary: '为编码 Agent 提供需求澄清、开发、验证等标准化流程。',
      sources: [{ sourceType: 'topic', term: 'ai-agents', configId: 'cfg-topic', lastSeenAt: now, hitCount: 1 }],
      evaluation: { id: 'ev-superpowers', evaluator: '李明', operator: '郑睿涛', cooperUrl, startedAt: '2026-09-09T03:00:00Z' },
    }),
    makeTool('eval-need-cooper', 'internal/need-cooper', {
      name: 'Need Cooper',
      status: 'evaluating',
      currentSummary: '用于验证测评中补充 Cooper 文档的工具。',
      sources: [{ sourceType: 'manual', term: 'internal/need-cooper', lastSeenAt: now, hitCount: 1 }],
      evaluation: { id: 'ev-need-cooper', evaluator: '赵敏', operator: '郑睿涛', startedAt: '2026-09-08T03:00:00Z' },
    }),
    makeTool('eval-batch-alpha', 'batch-eval/alpha', {
      name: 'Batch Alpha',
      status: 'evaluating',
      currentSummary: '用于批量不纳入验证的工具 Alpha。',
      sources: [{ sourceType: 'keyword', term: 'batch', configId: 'cfg-keyword', lastSeenAt: now, hitCount: 1 }],
      evaluation: { id: 'ev-batch-alpha', evaluator: '钱宁', operator: '郑睿涛', cooperUrl, startedAt: '2026-09-07T03:00:00Z' },
    }),
    makeTool('eval-batch-beta', 'batch-eval/beta', {
      name: 'Batch Beta',
      status: 'evaluating',
      currentSummary: '用于批量不纳入验证的工具 Beta。',
      sources: [{ sourceType: 'keyword', term: 'batch', configId: 'cfg-keyword', lastSeenAt: now, hitCount: 1 }],
      evaluation: { id: 'ev-batch-beta', evaluator: '钱宁', operator: '郑睿涛', cooperUrl, startedAt: '2026-09-07T03:00:00Z' },
    }),
    makeTool('eval-include-alpha', 'batch-include/alpha', {
      name: 'Batch Include Alpha',
      status: 'evaluating',
      currentSummary: '用于批量纳入验证的工具 Alpha。',
      sources: [{ sourceType: 'keyword', term: 'batch include', configId: 'cfg-keyword', lastSeenAt: now, hitCount: 1 }],
      evaluation: { id: 'ev-include-alpha', evaluator: '钱宁', operator: '郑睿涛', cooperUrl, startedAt: '2026-09-07T03:00:00Z' },
    }),
    makeTool('eval-include-beta', 'batch-include/beta', {
      name: 'Batch Include Beta',
      status: 'evaluating',
      currentSummary: '用于批量纳入验证的工具 Beta。',
      sources: [{ sourceType: 'manual', term: 'batch-include/beta', lastSeenAt: now, hitCount: 1 }],
      evaluation: { id: 'ev-include-beta', evaluator: '钱宁', operator: '郑睿涛', startedAt: '2026-09-07T03:00:00Z' },
    }),
  ]

  const includedBase = [
    makeTool('team-claude', 'anthropics/claude-code', {
      name: 'Claude Code',
      status: 'included',
      finalSummary: '团队复杂代码任务首选，用于需求实现、调试和代码审查。',
      currentSummary: '团队复杂代码任务首选，用于需求实现、调试和代码审查。',
      currentSummarySource: 'final',
      stars: 12_000,
      stars7d: 700,
      includedAt: '2026-09-01T03:00:00Z',
      sources: [{ sourceType: 'keyword', term: 'coding agent', configId: 'cfg-keyword', lastSeenAt: now, hitCount: 1 }],
      evaluation: { id: 'ev-claude', evaluator: '王静', operator: '郑睿涛', cooperUrl, startedAt: '2026-08-30T03:00:00Z', completedAt: '2026-09-01T03:00:00Z', result: 'included' },
    }),
    makeTool('team-context7', 'upstash/context7', {
      name: 'Context7',
      status: 'included',
      finalSummary: '为编码 Agent 补充最新官方文档上下文，减少过期 API 用法。',
      currentSummary: '为编码 Agent 补充最新官方文档上下文，减少过期 API 用法。',
      currentSummarySource: 'final',
      stars: 8_000,
      stars7d: 500,
      includedAt: '2026-09-02T03:00:00Z',
      sources: [{ sourceType: 'topic', term: 'model-context-protocol', configId: 'cfg-topic', lastSeenAt: now, hitCount: 1 }],
      evaluation: { id: 'ev-context7', evaluator: '李明', operator: '郑睿涛', cooperUrl, startedAt: '2026-08-30T03:00:00Z', completedAt: '2026-09-02T03:00:00Z', result: 'included' },
    }),
    makeTool('team-playwright', 'microsoft/playwright-mcp', {
      name: 'Playwright MCP',
      status: 'included',
      finalSummary: '团队网页调试和浏览器操作任务的标准 MCP 工具。',
      currentSummary: '团队网页调试和浏览器操作任务的标准 MCP 工具。',
      currentSummarySource: 'final',
      stars: 9_000,
      stars7d: 300,
      includedAt: '2026-09-03T03:00:00Z',
      sources: [{ sourceType: 'manual', term: 'microsoft/playwright-mcp', lastSeenAt: now, hitCount: 1 }],
      evaluation: { id: 'ev-playwright', evaluator: '周恺', operator: '郑睿涛', cooperUrl, startedAt: '2026-08-30T03:00:00Z', completedAt: '2026-09-03T03:00:00Z', result: 'included' },
    }),
  ]
  const includedCount = Math.max(options.includedCount ?? includedBase.length, includedBase.length)
  const generatedIncluded = Array.from({ length: includedCount - includedBase.length }, (_, index) => makeTool(`team-batch-${index}`, `team-owner-${index}/repo-${index}`, {
    name: `Team Repo ${index}`,
    status: 'included',
    finalSummary: `团队工具分页与批量维护验证说明 ${index}`,
    currentSummary: `团队工具分页与批量维护验证说明 ${index}`,
    currentSummarySource: 'final',
    stars: 20_000 + index,
    stars7d: index,
    includedAt: '2026-09-04T03:00:00Z',
    sources: [{ sourceType: 'keyword', term: 'team batch', configId: 'cfg-keyword', lastSeenAt: now, hitCount: 1 }],
    evaluation: { id: `ev-team-${index}`, evaluator: '王静', operator: '郑睿涛', cooperUrl, startedAt: '2026-08-30T03:00:00Z', completedAt: '2026-09-04T03:00:00Z', result: 'included' },
  }))

  return {
    calls: [],
    configs: [
      makeConfig('cfg-keyword', '流程图工具', 'keyword', ['flow chart', 'drawio'], 'manual', true),
      makeConfig('cfg-topic', '上线验证-Topic', 'topic', ['ai-agents'], 'weekly', true),
      makeConfig('cfg-browser', '浏览器操作工具', 'keyword', ['browser agent'], 'weekly', true),
      makeConfig('cfg-disabled', 'MCP 工具', 'topic', ['model-context-protocol'], 'weekly', false),
    ],
    tools: [...discoveredBase, ...generatedDiscovered, ...evaluating, ...includedBase, ...generatedIncluded],
  }
}

async function installMockRoutes(page: Page, state: MockState): Promise<MockState> {
  await page.route(/\/api\/(public|tools)(\/|\?|$)/, async (route) => {
    try {
      const request = route.request()
      const url = new URL(request.url())
      const path = url.pathname
      const body = request.method() === 'POST' ? postBody(request) : undefined
      state.calls.push({ path, body })

    if (path === '/api/public/version') {
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ apiVersion: '1.1.0', skillVersion: '0.1.0', updatedAt: now, changelogUrl: '', recentChanges: [] }),
      })
      return
    }

    if (path === '/api/tools/configs' && request.method() === 'GET') {
      await ok(route, listConfigs(state, url))
      return
    }

    if (path === '/api/tools/configs' && request.method() === 'POST') {
      const input = body as Partial<DiscoveryConfig> & { actor?: string }
      if (!input.actor) {
        await fail(route, 'actor is required')
        return
      }
      let config = state.configs.find((item) => item.id === input.id)
      if (config) {
        Object.assign(config, {
          name: input.name || config.name,
          method: input.method || config.method,
          terms: input.terms || config.terms,
          triggerMode: input.triggerMode || config.triggerMode,
          enabled: input.enabled ?? config.enabled,
          updatedBy: input.actor,
          updatedAt: now,
        })
      } else {
        config = makeConfig(`cfg-${state.configs.length + 1}`, input.name || '未命名配置', input.method || 'keyword', input.terms || [], input.triggerMode || 'manual', input.enabled ?? true, input.actor)
        state.configs.push(config)
      }
      await ok(route, { config })
      return
    }

    if (path === '/api/tools/configs/enable') {
      const input = body as { id: string; enabled: boolean; actor?: string }
      if (!input.actor) return fail(route, 'actor is required')
      const config = configByID(state, input.id)
      config.enabled = input.enabled
      config.updatedBy = input.actor
      config.updatedAt = now
      await ok(route, { config })
      return
    }

    if (path === '/api/tools/configs/delete') {
      const input = body as { id: string; actor?: string }
      if (!input.actor) return fail(route, 'actor is required')
      state.configs = state.configs.filter((item) => item.id !== input.id)
      await ok(route, { deleted: true })
      return
    }

    if (path === '/api/tools/configs/run') {
      const input = body as { id: string; actor?: string }
      if (!input.actor) return fail(route, 'actor is required')
      const config = configByID(state, input.id)
      config.lastRunStatus = 'running'
      config.lastRunAt = now
      config.lastPauseRequested = false
      config.lastResultCount = 0
      config.lastPagesScanned = 2
      await ok(route, { run: runPayload('running', false) })
      return
    }

    if (path === '/api/tools/configs/pause') {
      const input = body as { id: string; actor?: string }
      if (!input.actor) return fail(route, 'actor is required')
      const config = configByID(state, input.id)
      config.lastRunStatus = 'paused'
      config.lastPauseRequested = true
      await ok(route, { run: runPayload('paused', true) })
      return
    }

    if (path === '/api/tools/configs/resume') {
      const input = body as { id: string; actor?: string }
      if (!input.actor) return fail(route, 'actor is required')
      const config = configByID(state, input.id)
      config.lastRunStatus = 'running'
      config.lastPauseRequested = false
      await ok(route, { run: runPayload('running', false) })
      return
    }

    if (path === '/api/tools/items') {
      await ok(route, listTools(state, url))
      return
    }

    if (path === '/api/tools/summaries/url-key') {
      await ok(route, {
        toolId: (body as { toolId: string }).toolId,
        urlKey: (body as { urlKey: string }).urlKey,
        summary: '用于深度研究、资料整理和长文生成的中文说明。',
        source: 'url_key',
        translated: true,
      })
      return
    }

    if (path === '/api/tools/manual/preview') {
      const repo = parseRepoURL((body as { repoUrl: string }).repoUrl)
      await ok(route, {
        repository: {
          nodeId: `preview-${repo.fullName}`,
          owner: repo.owner,
          repo: repo.repo,
          fullName: repo.fullName,
          url: `https://github.com/${repo.fullName}`,
          name: previewName(repo.repo),
          description: '仓库预览信息',
          stars: 100,
          forks: 10,
          openIssues: 1,
          topics: ['ai-agent'],
          pushedAt: now,
        },
      })
      return
    }

    if (path === '/api/tools/manual/add') {
      const input = body as { repoUrl: string; actor?: string }
      if (!input.actor) return fail(route, 'actor is required')
      const repo = parseRepoURL(input.repoUrl)
      const existing = state.tools.find((tool) => tool.githubFullName === repo.fullName)
      const tool = existing ?? makeTool(`manual-${Date.now()}`, repo.fullName, {
        name: previewName(repo.repo),
        status: 'discovered',
        currentSummary: '手动添加后的工具说明。',
        sources: [{ sourceType: 'manual', term: repo.fullName, lastSeenAt: now, hitCount: 1 }],
      })
      if (!existing) state.tools.push(tool)
      await ok(route, { tool, created: !existing, duplicate: Boolean(existing) })
      return
    }

    if (path === '/api/tools/evaluations/start') {
      const input = body as { toolId: string; evaluator: string; operator: string; cooperUrl?: string }
      const tool = toolByID(state, input.toolId)
      tool.status = 'evaluating'
      tool.evaluation = { id: `ev-${input.toolId}`, evaluator: input.evaluator, operator: input.operator, cooperUrl: input.cooperUrl, startedAt: now }
      tool.updatedAt = now
      await ok(route, { evaluation: tool.evaluation })
      return
    }

    if (path === '/api/tools/evaluations/cooper-url') {
      const input = body as { toolId: string; cooperUrl: string; operator: string }
      const tool = toolByID(state, input.toolId)
      tool.evaluation = tool.evaluation ?? { id: `ev-${input.toolId}`, evaluator: '待填写', operator: input.operator, startedAt: now }
      tool.evaluation.cooperUrl = input.cooperUrl
      tool.evaluation.operator = input.operator
      await ok(route, { evaluation: tool.evaluation })
      return
    }

    if (path === '/api/tools/evaluations/finish') {
      const input = body as { toolId: string; result: 'included' | 'excluded'; operator: string; finalSummary?: string; notIncludedReason?: string }
      const tool = toolByID(state, input.toolId)
      if (input.result === 'included') {
        tool.status = 'included'
        tool.includedAt = now
        tool.finalSummary = input.finalSummary || tool.currentSummary
        tool.currentSummary = tool.finalSummary
        tool.currentSummarySource = 'final'
      } else {
        tool.status = 'excluded'
        tool.excludedAt = now
        tool.excludedStage = 'evaluating'
        tool.excludedReason = input.notIncludedReason
      }
      if (tool.evaluation) {
        tool.evaluation.operator = input.operator
        tool.evaluation.result = input.result
        tool.evaluation.completedAt = now
      }
      await ok(route, { completed: true })
      return
    }

    if (path === '/api/tools/discovered/exclude') {
      const input = body as { toolId: string; reason: string; operator: string }
      const tool = toolByID(state, input.toolId)
      tool.status = 'excluded'
      tool.excludedStage = 'discovered'
      tool.excludedReason = input.reason
      tool.updatedAt = now
      await ok(route, { excluded: true })
      return
    }

    if (path === '/api/tools/team/import') {
      const input = body as { repoUrl: string; operator: string; cooperUrl?: string; finalSummary?: string }
      if (!input.operator) return fail(route, 'actor is required')
      const repo = parseRepoURL(input.repoUrl)
      const existing = state.tools.find((tool) => tool.githubFullName === repo.fullName)
      const tool = existing ?? makeTool(`team-manual-${Date.now()}`, repo.fullName, {
        name: previewName(repo.repo),
        status: 'included',
        sources: [{ sourceType: 'manual', term: repo.fullName, lastSeenAt: now, hitCount: 1 }],
      })
      tool.status = 'included'
      tool.finalSummary = input.finalSummary || tool.finalSummary || '手动导入团队工具。'
      tool.currentSummary = tool.finalSummary
      tool.currentSummarySource = 'final'
      tool.includedAt = now
      tool.evaluation = {
        id: tool.evaluation?.id || `ev-${tool.id}`,
        evaluator: tool.evaluation?.evaluator || input.operator,
        operator: input.operator,
        cooperUrl: input.cooperUrl,
        startedAt: now,
        completedAt: now,
        result: 'included',
      }
      if (!existing) state.tools.push(tool)
      await ok(route, { tool, created: !existing, duplicate: Boolean(existing) })
      return
    }

    if (path === '/api/tools/team/update') {
      const input = body as { toolId: string; operator?: string; cooperUrl?: string; finalSummary?: string }
      if (!input.operator) return fail(route, 'actor is required')
      const tool = toolByID(state, input.toolId)
      tool.finalSummary = input.finalSummary || tool.finalSummary
      tool.currentSummary = tool.finalSummary
      tool.currentSummarySource = 'final'
      tool.evaluation = tool.evaluation ?? { id: `ev-${input.toolId}`, evaluator: input.operator, operator: input.operator, startedAt: now, result: 'included' }
      tool.evaluation.operator = input.operator
      if (input.cooperUrl) tool.evaluation.cooperUrl = input.cooperUrl
      await ok(route, { updated: true })
      return
    }

    if (path === '/api/tools/team/delete') {
      const input = body as { toolId: string; operator?: string; reason?: string }
      if (!input.operator) return fail(route, 'actor is required')
      if (!input.reason) return fail(route, 'reason is required')
      const tool = toolByID(state, input.toolId)
      tool.status = 'excluded'
      tool.excludedAt = now
      tool.excludedReason = input.reason
      await ok(route, { deleted: true })
      return
    }

      await ok(route, {})
    } catch (error) {
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({
          errno: 500000,
          errmsg: error instanceof Error ? error.message : String(error),
          data: { class: 'mock_route_error' },
        }),
      })
    }
  })
  return state
}

function makeConfig(id: string, name: string, method: DiscoveryMethod, terms: string[], triggerMode: TriggerMode, enabled: boolean, actor = '郑睿涛'): DiscoveryConfig {
  return {
    id,
    name,
    method,
    terms,
    triggerMode,
    enabled,
    lastResultCount: 0,
    lastPagesScanned: 0,
    lastPauseRequested: false,
    createdBy: actor,
    updatedBy: actor,
    createdAt: now,
    updatedAt: now,
  }
}

function makeTool(id: string, fullName: string, overrides: Partial<ToolItem> = {}): ToolItem {
  const [owner, repo] = fullName.split('/')
  return {
    id,
    name: repo,
    githubNodeId: `node-${id}`,
    githubOwner: owner,
    githubRepo: repo,
    githubFullName: fullName,
    githubUrl: `https://github.com/${fullName}`,
    description: 'A useful AI tool for workflow automation.',
    currentSummary: '用于 AI 工具链验证的默认中文说明。',
    currentSummarySource: 'github_description',
    stars: 100,
    forks: 10,
    openIssues: 1,
    stars7d: 0,
    topics: ['ai-agent'],
    licenseSpdx: 'MIT',
    defaultBranch: 'main',
    pushedAt: now,
    status: 'discovered',
    firstDiscoveredAt: now,
    lastDiscoveredAt: now,
    statusVersion: 1,
    summaryKey: `summary-${id}`,
    sources: [{ sourceType: 'keyword', term: 'ai agent', configId: 'cfg-keyword', lastSeenAt: now, hitCount: 1 }],
    createdAt: now,
    updatedAt: now,
    ...overrides,
  }
}

function listConfigs(state: MockState, url: URL) {
  const q = (url.searchParams.get('q') || '').toLowerCase()
  const methods = url.searchParams.getAll('method') as DiscoveryMethod[]
  const triggers = url.searchParams.getAll('triggerMode') as TriggerMode[]
  let items = [...state.configs]
  if (methods.length) items = items.filter((cfg) => methods.includes(cfg.method))
  if (triggers.length) items = items.filter((cfg) => triggers.includes(cfg.triggerMode))
  if (q) {
    items = items.filter((cfg) => `${cfg.name} ${cfg.terms.join(' ')}`.toLowerCase().includes(q))
  }
  return {
    count: items.length,
    enabledCount: items.filter((cfg) => cfg.enabled).length,
    disabledCount: items.filter((cfg) => !cfg.enabled).length,
    items,
  }
}

function listTools(state: MockState, url: URL) {
  const status = (url.searchParams.get('status') || 'discovered') as ToolStatus
  const sort = (url.searchParams.get('sort') || 'latest') as ToolSort
  const sources = url.searchParams.getAll('source') as ToolSourceType[]
  const q = (url.searchParams.get('q') || '').toLowerCase()
  const take = Number(url.searchParams.get('take') || 50)
  const offset = Number(url.searchParams.get('offset') || 0)
  let items = state.tools.filter((tool) => tool.status === status)
  if (sources.length) items = items.filter((tool) => tool.sources.some((source) => sources.includes(source.sourceType)))
  if (q) {
    items = items.filter((tool) => {
      const haystack = [
        tool.name,
        tool.githubFullName,
        tool.description,
        tool.currentSummary,
        tool.finalSummary,
        tool.evaluation?.evaluator,
        ...tool.sources.map((source) => `${source.sourceType} ${source.term}`),
      ].filter(Boolean).join(' ').toLowerCase()
      return haystack.includes(q)
    })
  }
  items = sortTools(items, sort)
  const count = items.length
  const pageItems = items.slice(offset, offset + take)
  return {
    count,
    page: Math.floor(offset / take) + 1,
    pageSize: take,
    offset,
    take,
    stats: statsFor(status, items),
    items: pageItems,
  }
}

function sortTools(items: ToolItem[], sort: ToolSort): ToolItem[] {
  return [...items].sort((a, b) => {
    if (sort === 'stars') return b.stars - a.stars
    if (sort === 'stars7d') return (b.stars7d ?? -1) - (a.stars7d ?? -1)
    return new Date(b.firstDiscoveredAt).getTime() - new Date(a.firstDiscoveredAt).getTime()
  })
}

function statsFor(status: ToolStatus, items: ToolItem[]) {
  return {
    status,
    keywordSourceCount: items.filter((tool) => tool.sources.some((source) => source.sourceType === 'keyword')).length,
    topicSourceCount: items.filter((tool) => tool.sources.some((source) => source.sourceType === 'topic')).length,
    manualSourceCount: items.filter((tool) => tool.sources.some((source) => source.sourceType === 'manual')).length,
    linkedEvaluationCount: items.filter((tool) => Boolean(tool.evaluation?.cooperUrl)).length,
    unlinkedEvaluationCount: items.filter((tool) => tool.evaluation && !tool.evaluation.cooperUrl).length,
    evaluatorCount: new Set(items.map((tool) => tool.evaluation?.evaluator).filter(Boolean)).size,
    latestUpdatedAt: now,
  }
}

function parseRepoURL(repoUrl: string) {
  const match = repoUrl.match(/github\.com\/([^/]+)\/([^/?#]+)/)
  if (!match) return { owner: 'unknown', repo: 'unknown', fullName: 'unknown/unknown' }
  return { owner: match[1], repo: match[2], fullName: `${match[1]}/${match[2]}` }
}

function configByID(state: MockState, id: string): DiscoveryConfig {
  const config = state.configs.find((item) => item.id === id)
  if (!config) throw new Error(`missing config: ${id}`)
  return config
}

function toolByID(state: MockState, id: string): ToolItem {
  const tool = state.tools.find((item) => item.id === id)
  if (!tool) throw new Error(`missing tool: ${id}`)
  return tool
}

function runPayload(status: RunStatus, pauseRequested: boolean) {
  return {
    id: 'run-1',
    status,
    resultCount: 0,
    newCount: 0,
    updatedCount: 0,
    skippedCount: 0,
    pagesScanned: 2,
    incompleteResults: false,
    truncated: false,
    pauseRequested,
    nextQueryIndex: 0,
    nextPage: 1,
    rateLimited: false,
  }
}

async function ok(route: Route, data: unknown) {
  await route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ errno: 0, errmsg: 'ok', data }),
  })
}

async function fail(route: Route, errmsg: string) {
  await route.fulfill({
    status: 400,
    contentType: 'application/json',
    body: JSON.stringify({ errno: 400001, errmsg, data: { class: 'validation_error' } }),
  })
}

function configCard(page: Page, name: string) {
  return page.locator('.tools-config-card').filter({ hasText: name })
}

function firstToolName(page: Page) {
  return page.locator('.tools-data-table tbody tr').first().locator('.tools-tool-name')
}

function toolRow(page: Page, name: string) {
  return page.locator('.tools-data-table tbody tr').filter({ hasText: name }).first()
}

function lastCall(state: MockState, path: string) {
  return [...state.calls].reverse().find((call) => call.path === path && call.body !== undefined)
    ?? [...state.calls].reverse().find((call) => call.path === path)
}

async function runSearch(page: Page, placeholder: string, value: string) {
  const input = page.getByPlaceholder(placeholder)
  await input.fill(value)
  await input.press('Enter')
}

async function resetToolPage(page: Page, url: string) {
  await page.goto(url)
  await page.reload({ waitUntil: 'domcontentloaded' })
  await expect(page.locator('.tools-page')).toBeVisible()
}

function postBody(request: Request) {
  const raw = request.postData()
  if (!raw) return undefined
  try {
    return JSON.parse(raw)
  } catch {
    return undefined
  }
}

function sourceFilter(page: Page) {
  return page.getByLabel('来源', { exact: true })
}

function toolName(page: Page, name: string) {
  return page.locator('.tools-tool-name').filter({ hasText: name }).first()
}

function previewName(repo: string) {
  if (repo === 'skills') return 'Anthropic Skills'
  if (repo === 'claude-code') return 'Claude Code'
  return repo
}
