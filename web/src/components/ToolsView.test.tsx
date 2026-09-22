import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { act } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ToolsView } from './ToolsView'

const now = '2026-09-10T02:00:00Z'

const baseTool = {
  id: 'tool1',
  name: 'browser-agent',
  githubNodeId: 'node1',
  githubOwner: 'openai',
  githubRepo: 'browser-agent',
  githubFullName: 'openai/browser-agent',
  githubUrl: 'https://github.com/openai/browser-agent',
  description: 'A browser automation agent',
  currentSummary: '用于浏览器自动化的 AI agent 工具',
  currentSummarySource: 'github_description',
  stars: 321,
  forks: 12,
  openIssues: 4,
  topics: ['browser-agent'],
  purposeTags: ['浏览器操作'],
  purposeTagsManuallySet: false,
  status: 'discovered',
  firstDiscoveredAt: now,
  lastDiscoveredAt: now,
  statusVersion: 1,
  summaryKey: 'summary-key-browser-agent',
  sources: [{ sourceType: 'manual', term: 'openai/browser-agent', lastSeenAt: now, hitCount: 1 }],
  createdAt: now,
  updatedAt: now,
}

function makeTool(id: string, fullName: string, overrides: Record<string, unknown> = {}) {
  const [owner, repo] = fullName.split('/')
  return {
    ...baseTool,
    id,
    name: repo,
    githubNodeId: `node-${id}`,
    githubOwner: owner,
    githubRepo: repo,
    githubFullName: fullName,
    githubUrl: `https://github.com/${fullName}`,
    sources: [{ sourceType: 'manual', term: fullName, lastSeenAt: now, hitCount: 1 }],
    ...overrides,
  }
}

function ok(data: unknown) {
  return Promise.resolve({ ok: true, json: async () => ({ errno: 0, errmsg: 'ok', data }) })
}

function toolList(status: string, items: unknown[], overrides: Record<string, unknown> = {}) {
  const typedItems = items as Array<{ sources?: Array<{ sourceType: string; term: string }>; purposeTags?: string[] }>
  const sourceCount = (sourceType: string) => typedItems.filter((item) => item.sources?.some((source) => source.sourceType === sourceType)).length
  const keywords = Array.from(new Set(typedItems.flatMap((item) => item.sources?.filter((source) => source.sourceType === 'keyword').map((source) => source.term) ?? []))).sort()
  return {
    count: items.length,
    page: 1,
    pageSize: 50,
    offset: 0,
    take: 50,
    stats: {
      status,
      keywordSourceCount: sourceCount('keyword'),
      topicSourceCount: sourceCount('topic'),
      manualSourceCount: sourceCount('manual'),
      linkedEvaluationCount: items.filter((item) => Boolean((item as { evaluation?: { cooperUrl?: string } }).evaluation?.cooperUrl)).length,
      unlinkedEvaluationCount: items.filter((item) => Boolean((item as { evaluation?: unknown }).evaluation)).length,
      evaluatorCount: 1,
      latestUpdatedAt: now,
      purposeTags: Array.from(new Set(typedItems.flatMap((item) => item.purposeTags ?? []))),
      keywords,
    },
    items,
    ...overrides,
  }
}

function configList(items: unknown[]) {
  return { count: items.length, enabledCount: items.length, disabledCount: 0, items }
}

describe('ToolsView', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    window.confirm = vi.fn(() => true)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('adds a named token, tests it, and saves its metadata', async () => {
    const preview = {
      id: 'token-1',
      name: '主账号',
      description: '日常工具发现',
      testUrl: 'https://api.github.com/rate_limit',
      index: 0,
      masked: '****abcd',
      last4: 'abcd',
      source: 'saved',
    }
    const emptySettings = {
      githubTokenCount: 0,
      githubTokens: [],
      defaultGitHubTokenCount: 0,
      defaultGitHubTokens: [],
      githubBaseUrl: '',
      githubTokenStrategy: 'round_robin',
      githubActiveTokenIndex: 0,
      includeDefaultGitHubTokens: false,
      githubMaxPages: 5,
      githubPerPage: 100,
      githubRequestIntervalMs: 2000,
      starSnapshotLimit: 200,
      updatedBy: 'alice',
    }
    const savedSettings = { ...emptySettings, githubTokenCount: 1, githubTokens: [preview], updatedBy: 'alice' }
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      if (url === '/api/tools/settings') return ok({ settings: savedSettings.githubTokens.length ? savedSettings : emptySettings })
      if (url === '/api/tools/settings/tokens/test') {
        return ok({ token: { ...preview, ok: true, limit: 5000, remaining: 4999 } })
      }
      if (url === '/api/tools/settings/tokens') return ok({ token: preview, settings: savedSettings })
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="accounts" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByRole('heading', { name: '添加 Token' })).toBeInTheDocument())
    fireEvent.change(screen.getByLabelText('Token 名称'), { target: { value: '主账号' } })
    fireEvent.change(screen.getByLabelText('Token 描述'), { target: { value: '日常工具发现' } })
    fireEvent.change(screen.getByLabelText('Token 值'), { target: { value: 'ghp_secret' } })
    fireEvent.change(screen.getByLabelText('测试链接'), { target: { value: 'https://api.github.com/rate_limit' } })
    fireEvent.click(screen.getByRole('button', { name: '测试 Token' }))

    await waitFor(() => expect(screen.getByText('Token 可用，剩余额度 4999/5000')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: '保存 Token' }))

    await waitFor(() => expect(screen.getByText('Token 已保存：主账号')).toBeInTheDocument())
    expect(fetchMock.mock.calls.some((call) => call[0] === '/api/tools/settings/tokens/test')).toBe(true)
    const saveCall = fetchMock.mock.calls.find((call) => call[0] === '/api/tools/settings/tokens')
    expect(JSON.parse(String(saveCall?.[1]?.body))).toMatchObject({
      name: '主账号',
      description: '日常工具发现',
      token: 'ghp_secret',
      testUrl: 'https://api.github.com/rate_limit',
      actor: 'alice',
    })
  })

  it('creates a discovery config from the config page', async () => {
    const config = {
      id: 'cfg1',
      name: '浏览器操作工具发现',
      method: 'keyword',
      terms: ['browser agent'],
      triggerMode: 'manual',
      enabled: true,
      lastResultCount: 0,
      lastFailureReason: '{"message":"You have exceeded a secondary rate limit."}',
      createdBy: 'alice',
      updatedBy: 'alice',
      createdAt: now,
      updatedAt: now,
    }
    const fetchMock = vi.fn((url: string, init?: RequestInit) => {
      if (url.startsWith('/api/tools/configs') && init?.method === 'POST') return ok({ config })
      if (url.startsWith('/api/tools/configs')) {
        return ok({ count: 1, enabledCount: 1, disabledCount: 0, items: [config] })
      }
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="configs" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('浏览器操作工具发现')).toBeInTheDocument())
    expect(screen.getByText('提示：GitHub 暂时限流，请稍后再试')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /新增配置/ }))
    fireEvent.change(screen.getByLabelText('配置名称'), { target: { value: '浏览器操作工具发现' } })
    fireEvent.change(screen.getByLabelText('检索词'), { target: { value: 'browser agent' } })
    fireEvent.change(screen.getByLabelText('操作人'), { target: { value: 'alice' } })
    fireEvent.click(screen.getByRole('button', { name: '创建配置' }))

    await waitFor(() => expect(screen.getByText('配置已创建')).toBeInTheDocument())
    expect(fetchMock.mock.calls.some((call) => call[0] === '/api/tools/configs' && call[1]?.method === 'POST')).toBe(true)
  })

  it('shows missing required-field errors inside the new config modal', async () => {
    const fetchMock = vi.fn((url: string, init?: RequestInit) => {
      if (url === '/api/tools/configs' && init?.method === 'POST') {
        throw new Error('create config should be blocked by client validation')
      }
      if (url.startsWith('/api/tools/configs')) return ok(configList([]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="configs" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByRole('heading', { name: '发现配置' })).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: /新增配置/ }))
    const dialog = screen.getByRole('dialog', { name: '新增发现配置' })
    fireEvent.click(screen.getByRole('button', { name: '创建配置' }))

    await waitFor(() => expect(dialog).toHaveTextContent('请先填写操作人'))
    expect(document.querySelector('.tools-error')).toBeNull()
    expect(fetchMock.mock.calls.some((call) => call[0] === '/api/tools/configs' && call[1]?.method === 'POST')).toBe(false)
  })

  it('filters discovery configs by method and trigger mode', async () => {
    const config = {
      id: 'cfg1',
      name: '浏览器操作工具发现',
      method: 'keyword',
      terms: ['browser agent'],
      triggerMode: 'manual',
      enabled: true,
      lastResultCount: 0,
      createdBy: 'alice',
      updatedBy: 'alice',
      createdAt: now,
      updatedAt: now,
    }
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      if (String(url).startsWith('/api/tools/configs')) return ok(configList([config]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="configs" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('浏览器操作工具发现')).toBeInTheDocument())

    fireEvent.change(screen.getByLabelText('发现方式'), { target: { value: 'keyword' } })
    await waitFor(() => {
      const urls = fetchMock.mock.calls.map((call) => String(call[0]))
      expect(urls.some((url) => url.includes('method=keyword'))).toBe(true)
    })

    fireEvent.change(screen.getByLabelText('触发方式'), { target: { value: 'manual' } })
    await waitFor(() => {
      const urls = fetchMock.mock.calls.map((call) => String(call[0]))
      expect(urls.some((url) => url.includes('method=keyword') && url.includes('triggerMode=manual'))).toBe(true)
    })
  })

  it('uses the saved config operator when config actions are clicked with an empty actor field', async () => {
    const config = {
      id: 'cfg1',
      name: '浏览器操作工具发现',
      method: 'keyword',
      terms: ['browser agent'],
      triggerMode: 'manual',
      enabled: true,
      lastResultCount: 0,
      createdBy: 'alice',
      updatedBy: 'alice',
      createdAt: now,
      updatedAt: now,
    }
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      if (url === '/api/tools/configs/enable') return ok({ config: { ...config, enabled: false } })
      if (url === '/api/tools/configs/delete') return ok({ deleted: true })
      if (url === '/api/tools/configs' && _init?.method === 'POST') return ok({ config })
      if (String(url).startsWith('/api/tools/configs')) return ok(configList([config]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="configs" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('浏览器操作工具发现')).toBeInTheDocument())

    const disableButton = screen.getByRole('button', { name: '停用' })
    expect(disableButton).toHaveTextContent('⏻')
    fireEvent.click(disableButton)
    await waitFor(() => expect(screen.getByText('配置已停用')).toBeInTheDocument())
    const enableCall = fetchMock.mock.calls.find((call) => call[0] === '/api/tools/configs/enable')
    expect(JSON.parse(String(enableCall?.[1]?.body)).actor).toBe('alice')

    fireEvent.click(screen.getByRole('button', { name: '编辑' }))
    fireEvent.click(screen.getByRole('button', { name: '保存配置' }))
    await waitFor(() => expect(screen.getByText('配置已更新')).toBeInTheDocument())
    const saveCall = fetchMock.mock.calls.find((call) => call[0] === '/api/tools/configs' && call[1]?.method === 'POST')
    expect(JSON.parse(String(saveCall?.[1]?.body)).actor).toBe('alice')

    fireEvent.click(screen.getByRole('button', { name: '删除' }))
    await waitFor(() => expect(screen.getByText('配置已删除')).toBeInTheDocument())
    const deleteCall = fetchMock.mock.calls.find((call) => call[0] === '/api/tools/configs/delete')
    expect(JSON.parse(String(deleteCall?.[1]?.body)).actor).toBe('alice')
  })

  it('pauses a config when the run button is clicked again after starting', async () => {
    const config = {
      id: 'cfg1',
      name: '浏览器操作工具发现',
      method: 'keyword',
      terms: ['browser agent'],
      triggerMode: 'manual',
      enabled: true,
      lastResultCount: 0,
      createdBy: 'alice',
      updatedBy: 'alice',
      createdAt: now,
      updatedAt: now,
    }
    let resolveRun: ((value: unknown) => void) | undefined
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      if (url === '/api/tools/configs/run') {
        return new Promise((resolve) => {
          resolveRun = () => resolve(ok({
            run: {
              id: 'run1',
              status: 'running',
              resultCount: 0,
              newCount: 0,
              updatedCount: 0,
              skippedCount: 0,
              pagesScanned: 0,
              incompleteResults: false,
              truncated: false,
              pauseRequested: false,
              nextQueryIndex: 0,
              nextPage: 1,
              rateLimited: false,
            },
          }))
        })
      }
      if (url === '/api/tools/configs/pause') {
        return ok({
          run: {
            id: 'run1',
            status: 'running',
            resultCount: 0,
            newCount: 0,
            updatedCount: 0,
            skippedCount: 0,
            pagesScanned: 0,
            incompleteResults: false,
            truncated: false,
            pauseRequested: true,
            nextQueryIndex: 0,
            nextPage: 1,
            rateLimited: false,
          },
        })
      }
      if (String(url).startsWith('/api/tools/configs')) return ok(configList([config]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="configs" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('浏览器操作工具发现')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: '执行' }))
    await waitFor(() => expect(screen.getByRole('button', { name: '暂停' })).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: '暂停' }))

    await waitFor(() => expect(screen.getByText('已请求暂停，当前请求完成后会暂停')).toBeInTheDocument())
    expect(fetchMock.mock.calls.filter((call) => call[0] === '/api/tools/configs/run')).toHaveLength(1)
    expect(fetchMock.mock.calls.filter((call) => call[0] === '/api/tools/configs/pause')).toHaveLength(1)

    resolveRun?.({})
    await waitFor(() => expect(screen.getByText('已开始执行，刷新页面不会中断')).toBeInTheDocument())
  })

  it('pauses persisted running configs after reload', async () => {
    const config = {
      id: 'cfg1',
      name: '浏览器操作工具发现',
      method: 'keyword',
      terms: ['browser agent'],
      triggerMode: 'manual',
      enabled: true,
      lastRunAt: now,
      lastRunStatus: 'running',
      lastResultCount: 0,
      lastPagesScanned: 2,
      lastPauseRequested: false,
      createdBy: 'alice',
      updatedBy: 'alice',
      createdAt: now,
      updatedAt: now,
    }
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      if (url === '/api/tools/configs/pause') return ok({
        run: {
          id: 'run1',
          status: 'running',
          resultCount: 0,
          newCount: 0,
          updatedCount: 0,
          skippedCount: 0,
          pagesScanned: 2,
          incompleteResults: false,
          truncated: false,
          pauseRequested: true,
          nextQueryIndex: 0,
          nextPage: 3,
          rateLimited: false,
        },
      })
      if (String(url).startsWith('/api/tools/configs')) return ok(configList([config]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="configs" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByRole('button', { name: '暂停' })).toBeInTheDocument())
    expect(screen.getByRole('button', { name: '暂停' })).toHaveTextContent('Ⅱ')
    expect(screen.getByText('运行中 · 已扫 2 页 · 结果 0')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '暂停' }))

    await waitFor(() => expect(screen.getByText('已请求暂停，当前请求完成后会暂停')).toBeInTheDocument())
    expect(fetchMock.mock.calls.filter((call) => call[0] === '/api/tools/configs/run')).toHaveLength(0)
    expect(fetchMock.mock.calls.filter((call) => call[0] === '/api/tools/configs/pause')).toHaveLength(1)
  })

  it('resumes paused configs from the config list', async () => {
    const config = {
      id: 'cfg1',
      name: '浏览器操作工具发现',
      method: 'keyword',
      terms: ['browser agent'],
      triggerMode: 'manual',
      enabled: true,
      lastRunAt: now,
      lastRunStatus: 'paused',
      lastResultCount: 50,
      lastPagesScanned: 2,
      lastPauseRequested: false,
      createdBy: 'alice',
      updatedBy: 'alice',
      createdAt: now,
      updatedAt: now,
    }
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      if (url === '/api/tools/configs/resume') return ok({
        run: {
          id: 'run1',
          status: 'running',
          resultCount: 50,
          newCount: 10,
          updatedCount: 40,
          skippedCount: 0,
          pagesScanned: 2,
          incompleteResults: false,
          truncated: false,
          pauseRequested: false,
          nextQueryIndex: 0,
          nextPage: 3,
          rateLimited: false,
        },
      })
      if (String(url).startsWith('/api/tools/configs')) return ok(configList([config]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="configs" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByRole('button', { name: '继续' })).toBeInTheDocument())
    expect(screen.getByText('已暂停 · 已扫 2 页 · 结果 50')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '继续' }))

    await waitFor(() => expect(screen.getByText('已继续执行')).toBeInTheDocument())
    expect(fetchMock.mock.calls.filter((call) => call[0] === '/api/tools/configs/resume')).toHaveLength(1)
  })

  it('previews, adds and starts evaluation from discovered tools', async () => {
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      if (url === '/api/tools/manual/preview') {
        return ok({ repository: { nodeId: 'node1', owner: 'openai', repo: 'browser-agent', fullName: 'openai/browser-agent', url: 'https://github.com/openai/browser-agent', name: 'browser-agent', description: 'A browser automation agent', stars: 321, forks: 12, openIssues: 4, topics: [], purposeTags: ['浏览器操作'] } })
      }
      if (url === '/api/tools/manual/add') return ok({ tool: baseTool, created: true, duplicate: false })
      if (url === '/api/tools/evaluations/start') return ok({ evaluation: { id: 'eval1', evaluator: '张三', operator: '李四', startedAt: now } })
      if (String(url).startsWith('/api/tools/items')) return ok(toolList('discovered', [baseTool]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="discovered" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('browser-agent')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: /手动添加/ }))
    fireEvent.change(screen.getByPlaceholderText('https://github.com/owner/repo'), { target: { value: 'https://github.com/openai/browser-agent' } })
    fireEvent.change(screen.getByLabelText('操作人'), { target: { value: '李四' } })
    fireEvent.click(screen.getByRole('button', { name: '预览' }))
    await waitFor(() => expect(screen.getByRole('button', { name: '加入已发现' })).toBeEnabled())
    fireEvent.click(screen.getByRole('button', { name: '加入已发现' }))
    await waitFor(() => expect(screen.getByText('仓库已加入已发现列表')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: '开始测评' }))
    fireEvent.change(screen.getByLabelText('测评人'), { target: { value: '张三' } })
    const operatorInputs = screen.getAllByLabelText('操作人')
    fireEvent.change(operatorInputs[operatorInputs.length - 1], { target: { value: '李四' } })
    fireEvent.click(screen.getByRole('button', { name: '提交' }))

    await waitFor(() => expect(screen.getByText('已开始测评')).toBeInTheDocument())
    expect(fetchMock.mock.calls.map((call) => call[0])).toContain('/api/tools/evaluations/start')
  })

  it('filters and manually corrects purpose tags', async () => {
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      if (url === '/api/tools/purpose-tags') return ok({ tool: { ...baseTool, purposeTags: ['工作流自动化'], purposeTagsManuallySet: true } })
      if (String(url).startsWith('/api/tools/items')) return ok(toolList('discovered', [baseTool]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="discovered" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('浏览器操作')).toBeInTheDocument())

    fireEvent.change(screen.getByLabelText('用途'), { target: { value: '浏览器操作' } })
    await waitFor(() => {
      const urls = fetchMock.mock.calls.map((call) => String(call[0]))
      expect(urls.some((url) => url.includes('purposeTag=%E6%B5%8F%E8%A7%88%E5%99%A8%E6%93%8D%E4%BD%9C'))).toBe(true)
    })

    fireEvent.click(screen.getByRole('button', { name: '分类' }))
    fireEvent.change(screen.getByLabelText('操作人'), { target: { value: '李四' } })
    fireEvent.click(screen.getByRole('button', { name: '浏览器操作' }))
    fireEvent.change(screen.getByPlaceholderText('新增分类，如：会议纪要'), { target: { value: '工作流自动化' } })
    fireEvent.click(screen.getByRole('button', { name: '新增' }))
    fireEvent.click(screen.getByRole('button', { name: '提交' }))

    await waitFor(() => expect(screen.getByText('用途分类已更新')).toBeInTheDocument())
    const updateCall = fetchMock.mock.calls.find((call) => call[0] === '/api/tools/purpose-tags')
    expect(JSON.parse(String(updateCall?.[1]?.body))).toEqual({
      toolId: 'tool1',
      actor: '李四',
      purposeTags: ['工作流自动化'],
    })
  })

  it('filters discovered tools by keyword source', async () => {
    const browserTool = makeTool('tool-browser', 'openai/browser-agent', {
      name: 'Browser Agent',
      sources: [{ sourceType: 'keyword', term: 'browser agent', lastSeenAt: now, hitCount: 1 }],
    })
    const codingTool = makeTool('tool-coding', 'openai/coding-agent', {
      name: 'Coding Agent',
      sources: [{ sourceType: 'keyword', term: 'coding agent', lastSeenAt: now, hitCount: 1 }],
    })
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      if (String(url).startsWith('/api/tools/items')) return ok(toolList('discovered', [browserTool, codingTool]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="discovered" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('Browser Agent')).toBeInTheDocument())

    fireEvent.change(screen.getByLabelText('关键词'), { target: { value: 'browser agent' } })
    await waitFor(() => {
      const listRequest = fetchMock.mock.calls
        .map((call) => String(call[0]))
        .find((url) => url.startsWith('/api/tools/items') && new URL(url, 'http://localhost').searchParams.get('keyword') === 'browser agent')
      expect(listRequest).toBeDefined()
    })
  })

  it('clears a keyword filter when the source filter changes', async () => {
    const browserTool = makeTool('tool-browser', 'openai/browser-agent', {
      name: 'Browser Agent',
      sources: [{ sourceType: 'keyword', term: 'browser agent', lastSeenAt: now, hitCount: 1 }],
    })
    const manualTool = makeTool('tool-manual', 'openai/manual-agent', {
      name: 'Manual Agent',
      sources: [{ sourceType: 'manual', term: 'openai/manual-agent', lastSeenAt: now, hitCount: 1 }],
    })
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      if (String(url).startsWith('/api/tools/items')) return ok(toolList('discovered', [browserTool, manualTool]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="discovered" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('Browser Agent')).toBeInTheDocument())

    fireEvent.change(screen.getByLabelText('关键词'), { target: { value: 'browser agent' } })
    await waitFor(() => {
      const request = fetchMock.mock.calls
        .map((call) => String(call[0]))
        .find((url) => url.startsWith('/api/tools/items') && new URL(url, 'http://localhost').searchParams.get('keyword') === 'browser agent')
      expect(request).toBeDefined()
    })

    fireEvent.change(screen.getByLabelText('来源'), { target: { value: 'manual' } })
    await waitFor(() => {
      const request = fetchMock.mock.calls
        .map((call) => String(call[0]))
        .find((url) => {
          const parsed = new URL(url, 'http://localhost')
          return parsed.pathname === '/api/tools/items' && parsed.searchParams.get('source') === 'manual' && !parsed.searchParams.has('keyword')
        })
      expect(request).toBeDefined()
    })
    expect(screen.getByLabelText('关键词')).toHaveValue('all')
  })

  it('allows entering the number of tools shown for batch selection', async () => {
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      if (String(url).startsWith('/api/tools/items')) return ok(toolList('discovered', [baseTool]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="discovered" onSection={vi.fn()} />)
    const pageSize = await screen.findByLabelText('每页数量')
    expect(pageSize).toHaveAttribute('type', 'number')
    expect(pageSize).toHaveAttribute('min', '1')
    expect(pageSize).toHaveAttribute('max', '200')

    fireEvent.change(pageSize, { target: { value: '100' } })
    await waitFor(() => {
      const request = fetchMock.mock.calls
        .map((call) => String(call[0]))
        .find((url) => url.startsWith('/api/tools/items') && new URL(url, 'http://localhost').searchParams.get('take') === '100')
      expect(request).toBeDefined()
    })
  })

  it('restores the batch selection count input and step controls', async () => {
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      if (String(url).startsWith('/api/tools/items')) return ok(toolList('discovered', [baseTool], { count: 3 }))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="discovered" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('openai/browser-agent')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('checkbox', { name: '选择 openai/browser-agent' }))

    expect(screen.getByLabelText('选择数量')).toHaveValue(1)
    expect(screen.getByRole('button', { name: '增加一个选择' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '减少一个选择' })).toBeInTheDocument()
  })

  it('restores direct team inclusion and member review actions for each discovered tool', async () => {
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      if (url === '/api/tools/team/include') return ok({ included: true })
      if (url === '/api/tools/reviews/add') return ok({ review: { id: 'review1', reviewer: '张三', content: '适合团队浏览器自动化任务。', createdAt: now } })
      if (String(url).startsWith('/api/tools/items')) return ok(toolList('discovered', [baseTool]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="discovered" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('openai/browser-agent')).toBeInTheDocument())

    expect(screen.getByRole('button', { name: '加入团队工具' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '评价' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '评价' }))
    fireEvent.change(screen.getByLabelText('评价人'), { target: { value: '张三' } })
    fireEvent.change(screen.getByLabelText('团队评价'), { target: { value: '适合团队浏览器自动化任务。' } })
    fireEvent.click(screen.getByRole('button', { name: '提交' }))

    await waitFor(() => expect(screen.getByText('团队评价已提交')).toBeInTheDocument())
    expect(fetchMock.mock.calls.some((call) => call[0] === '/api/tools/reviews/add')).toBe(true)

    fireEvent.click(screen.getByRole('button', { name: '加入团队工具' }))
    fireEvent.click(screen.getByRole('button', { name: '提交' }))
    expect(screen.getByText('请填写操作人')).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('操作人'), { target: { value: '李四' } })
    fireEvent.change(screen.getByLabelText('Cooper 文档'), { target: { value: 'https://cooper.didichuxing.com/didocs/2' } })
    fireEvent.change(screen.getByLabelText('使用说明'), { target: { value: '团队直接使用它处理浏览器自动化任务。' } })
    fireEvent.click(screen.getByRole('button', { name: '提交' }))

    await waitFor(() => expect(screen.getByText('已加入团队工具')).toBeInTheDocument())
    const includeCall = fetchMock.mock.calls.find((call) => call[0] === '/api/tools/team/include')
    expect(JSON.parse(String(includeCall?.[1]?.body))).toMatchObject({
      toolId: 'tool1',
      operator: '李四',
      cooperUrl: 'https://cooper.didichuxing.com/didocs/2',
      finalSummary: '团队直接使用它处理浏览器自动化任务。',
      purposeTags: ['浏览器操作'],
    })

    fireEvent.click(screen.getByRole('button', { name: '编辑' }))
    fireEvent.change(screen.getByLabelText('操作人'), { target: { value: '王五' } })
    fireEvent.click(screen.getByRole('button', { name: '提交' }))
    await waitFor(() => expect(screen.getByText('工具信息已更新')).toBeInTheDocument())
    const editCall = fetchMock.mock.calls.find((call) => call[0] === '/api/tools/items/update')
    expect(JSON.parse(String(editCall?.[1]?.body))).toMatchObject({
      toolId: 'tool1',
      operator: '王五',
      purposeTags: ['浏览器操作'],
    })
  })

  it('links the real GitHub identity and preloads a Chinese summary once', async () => {
    const longDescription = 'AI research workspace with two modes. DeepSearch turns a query into a cited web report and Research Mode runs a multi-agent pipeline.'
    const tool = {
      ...baseTool,
      currentSummary: undefined,
      description: longDescription,
      summaryKey: 'url-key-1',
    }
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      if (url === '/api/tools/summaries/url-key') {
        return ok({ toolId: tool.id, urlKey: tool.summaryKey, summary: '用于完成深度研究和资料整理的 AI 工具。', source: `url_key:${tool.summaryKey}`, translated: true })
      }
      if (String(url).startsWith('/api/tools/items')) return ok(toolList('discovered', [tool]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="discovered" onSection={vi.fn()} />)
    const link = await screen.findByRole('link', { name: '打开 GitHub：openai/browser-agent' })
    expect(link).toHaveAttribute('href', 'https://github.com/openai/browser-agent')
    await waitFor(() => expect(screen.getByText('用于完成深度研究和资料整理的 AI 工具。')).toBeInTheDocument())
    const desc = screen.getByText('用于完成深度研究和资料整理的 AI 工具。')
    expect(desc).toHaveAttribute('title', '用于完成深度研究和资料整理的 AI 工具。')
    fireEvent.mouseEnter(desc)
    const summaryCalls = fetchMock.mock.calls.filter((call) => call[0] === '/api/tools/summaries/url-key')
    expect(summaryCalls).toHaveLength(1)
    const summaryCall = summaryCalls[0]
    expect(JSON.parse(String(summaryCall?.[1]?.body))).toEqual({ toolId: tool.id, urlKey: tool.summaryKey })
  })

  it('adds sorting and source filters to evaluating tools', async () => {
    const evaluatingTool = {
      ...baseTool,
      status: 'evaluating',
      evaluation: {
        id: 'eval1',
        evaluator: '张三',
        operator: '李四',
        startedAt: now,
      },
    }
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      if (String(url).startsWith('/api/tools/items')) return ok(toolList('evaluating', [evaluatingTool]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="evaluating" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getAllByText('张三').length).toBeGreaterThan(0))
    expect(screen.getByRole('button', { name: /当前 Stars/ })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /当前 Stars/ }))
    await waitFor(() => {
      const urls = fetchMock.mock.calls.map((call) => String(call[0]))
      expect(urls.some((url) => url.includes('status=evaluating') && url.includes('sort=stars'))).toBe(true)
    })

    fireEvent.change(screen.getByLabelText('来源'), { target: { value: 'manual' } })
    await waitFor(() => {
      const urls = fetchMock.mock.calls.map((call) => String(call[0]))
      expect(urls.some((url) => url.includes('source=manual') && !url.includes('source=keyword') && !url.includes('source=topic'))).toBe(true)
    })
  })

  it('finishes an evaluating tool as included', async () => {
    const evaluatingTool = {
      ...baseTool,
      status: 'evaluating',
      evaluation: {
        id: 'eval1',
        evaluator: '张三',
        operator: '李四',
        cooperUrl: 'https://cooper.didichuxing.com/didocs/1',
        startedAt: now,
      },
    }
    const fetchMock = vi.fn((url: string) => {
      if (url === '/api/tools/evaluations/finish') return ok({ completed: true })
      if (String(url).startsWith('/api/tools/items')) return ok(toolList('evaluating', [evaluatingTool]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="evaluating" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getAllByText('张三').length).toBeGreaterThan(0))

    fireEvent.click(screen.getByRole('button', { name: '✓ 纳入' }))
    fireEvent.change(screen.getByLabelText('操作人'), { target: { value: '李四' } })
    fireEvent.change(screen.getByLabelText('团队使用说明'), { target: { value: '它是一个浏览器自动化工具，解决网页任务验证问题，适合研发和测试场景。' } })
    fireEvent.click(screen.getByRole('button', { name: '提交' }))

    await waitFor(() => expect(screen.getByText('已纳入团队工具')).toBeInTheDocument())
    expect(fetchMock.mock.calls.map((call) => call[0])).toContain('/api/tools/evaluations/finish')
  })

  it('blocks evaluation include when operator is missing and clears the modal prompt', async () => {
    const evaluatingTool = {
      ...baseTool,
      status: 'evaluating',
      evaluation: {
        id: 'eval1',
        evaluator: '张三',
        operator: '李四',
        cooperUrl: 'https://cooper.didichuxing.com/didocs/1',
        startedAt: now,
      },
    }
    const fetchMock = vi.fn((url: string) => {
      if (url === '/api/tools/evaluations/finish') return ok({ completed: true })
      if (String(url).startsWith('/api/tools/items')) return ok(toolList('evaluating', [evaluatingTool]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="evaluating" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getAllByText('张三').length).toBeGreaterThan(0))

    vi.useFakeTimers()
    fireEvent.click(screen.getByRole('button', { name: '✓ 纳入' }))
    fireEvent.click(screen.getByRole('button', { name: '提交' }))

    expect(screen.getByText('请填写操作人')).toBeInTheDocument()
    expect(fetchMock.mock.calls.filter((call) => call[0] === '/api/tools/evaluations/finish')).toHaveLength(0)

    act(() => {
      vi.advanceTimersByTime(3000)
    })
    expect(screen.queryByText('请填写操作人')).not.toBeInTheDocument()
  })

  it('imports, edits and deletes team tools', async () => {
    const teamTool = {
      ...baseTool,
      status: 'included',
      finalSummary: '团队用它处理浏览器任务。',
      includedAt: now,
      evaluation: {
        id: 'eval1',
        evaluator: 'alice',
        operator: 'alice',
        cooperUrl: 'https://cooper.didichuxing.com/didocs/1',
        startedAt: now,
        completedAt: now,
        result: 'included',
        finalSummary: '团队用它处理浏览器任务。',
      },
    }
    const fetchMock = vi.fn((url: string) => {
      if (url === '/api/tools/manual/preview') {
        return ok({ repository: { nodeId: 'node1', owner: 'openai', repo: 'browser-agent', fullName: 'openai/browser-agent', url: 'https://github.com/openai/browser-agent', name: 'browser-agent', description: 'A browser automation agent', stars: 321, forks: 12, openIssues: 4, topics: [] } })
      }
      if (url === '/api/tools/team/import') return ok({ tool: teamTool, created: true, duplicate: false })
      if (url === '/api/tools/team/update') return ok({ updated: true })
      if (url === '/api/tools/team/delete') return ok({ deleted: true })
      if (String(url).startsWith('/api/tools/items')) return ok(toolList('included', [teamTool]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="team" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('browser-agent')).toBeInTheDocument())
    expect(screen.getByRole('button', { name: /当前 Stars/ })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /手动导入/ }))
    fireEvent.change(screen.getByPlaceholderText('https://github.com/owner/repo'), { target: { value: 'https://github.com/openai/browser-agent' } })
    fireEvent.change(screen.getByLabelText('操作人'), { target: { value: 'alice' } })
    fireEvent.change(screen.getByLabelText('Cooper 文档'), { target: { value: 'https://cooper.didichuxing.com/didocs/1' } })
    fireEvent.change(screen.getByLabelText('团队使用说明'), { target: { value: '团队用它处理浏览器任务。' } })
    fireEvent.click(screen.getByRole('button', { name: '预览' }))
    await waitFor(() => expect(screen.getByRole('button', { name: '加入团队工具' })).toBeEnabled())
    fireEvent.click(screen.getByRole('button', { name: '加入团队工具' }))
    await waitFor(() => expect(screen.getByText('工具已加入团队工具')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: '编辑' }))
    fireEvent.change(screen.getByLabelText('操作人'), { target: { value: 'bob' } })
    fireEvent.change(screen.getByLabelText('团队使用说明'), { target: { value: '更新后的团队说明。' } })
    fireEvent.click(screen.getByRole('button', { name: '提交' }))
    await waitFor(() => expect(screen.getByText('团队工具已更新')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: '删除' }))
    fireEvent.change(screen.getByLabelText('操作人'), { target: { value: 'carol' } })
    fireEvent.change(screen.getByLabelText('删除原因'), { target: { value: '团队不再使用' } })
    fireEvent.click(screen.getByRole('button', { name: '提交' }))
    await waitFor(() => expect(screen.getByText('团队工具已删除')).toBeInTheDocument())

    expect(fetchMock.mock.calls.map((call) => call[0])).toContain('/api/tools/team/import')
    expect(fetchMock.mock.calls.map((call) => call[0])).toContain('/api/tools/team/update')
    expect(fetchMock.mock.calls.map((call) => call[0])).toContain('/api/tools/team/delete')
  })

  it('batch starts and deletes discovered tools', async () => {
    const toolA = makeTool('tool-a', 'openai/browser-agent')
    const toolB = makeTool('tool-b', 'anthropic/skills')
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      if (url === '/api/tools/evaluations/start') return ok({ evaluation: { id: 'eval1', evaluator: '张三', operator: '李四', startedAt: now } })
      if (url === '/api/tools/discovered/exclude') return ok({ excluded: true })
      if (String(url).startsWith('/api/tools/items')) return ok(toolList('discovered', [toolA, toolB]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="discovered" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('anthropic/skills')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('checkbox', { name: '选择 openai/browser-agent' }))
    fireEvent.click(screen.getByRole('checkbox', { name: '选择 anthropic/skills' }))
    fireEvent.click(screen.getByRole('button', { name: '批量测试' }))
    fireEvent.change(screen.getByLabelText('测评人'), { target: { value: '张三' } })
    fireEvent.change(screen.getByLabelText('操作人'), { target: { value: '李四' } })
    const cooperInputs = screen.getAllByLabelText('Cooper 链接')
    fireEvent.change(cooperInputs[0], { target: { value: 'https://cooper.didichuxing.com/didocs/1' } })
    fireEvent.change(cooperInputs[1], { target: { value: 'https://cooper.didichuxing.com/didocs/2' } })
    fireEvent.click(screen.getByRole('button', { name: '提交' }))

    await waitFor(() => expect(screen.getByText('批量测评完成：2 个工具')).toBeInTheDocument())
    const startCalls = fetchMock.mock.calls.filter((call) => call[0] === '/api/tools/evaluations/start')
    expect(startCalls).toHaveLength(2)
    expect(JSON.parse(String(startCalls[0][1]?.body)).cooperUrl).toBe('https://cooper.didichuxing.com/didocs/1')
    expect(JSON.parse(String(startCalls[1][1]?.body)).cooperUrl).toBe('https://cooper.didichuxing.com/didocs/2')

    fireEvent.click(screen.getByRole('checkbox', { name: '选择 openai/browser-agent' }))
    fireEvent.click(screen.getByRole('checkbox', { name: '选择 anthropic/skills' }))
    fireEvent.click(screen.getByRole('button', { name: '批量删除' }))
    fireEvent.change(screen.getByLabelText('操作人'), { target: { value: '王五' } })
    fireEvent.change(screen.getByLabelText('不处理原因'), { target: { value: '不适合当前团队' } })
    fireEvent.click(screen.getByRole('button', { name: '提交' }))

    await waitFor(() => expect(screen.getByText('批量删除完成：2 个工具')).toBeInTheDocument())
    const deleteCalls = fetchMock.mock.calls.filter((call) => call[0] === '/api/tools/discovered/exclude')
    expect(deleteCalls).toHaveLength(2)
  })

  it('keeps selected tools across discovered list pages for batch actions', async () => {
    const toolA = makeTool('tool-a', 'openai/browser-agent')
    const toolB = makeTool('tool-b', 'anthropic/skills')
    const fetchMock = vi.fn((url: string, _init?: RequestInit) => {
      if (url === '/api/tools/evaluations/start') return ok({ evaluation: { id: 'eval1', evaluator: '张三', operator: '李四', startedAt: now } })
      if (String(url).startsWith('/api/tools/items')) {
        const parsed = new URL(url, 'http://localhost')
        const offset = parsed.searchParams.get('offset')
        if (offset === '50') return ok(toolList('discovered', [toolB], { count: 60, page: 2, offset: 50 }))
        return ok(toolList('discovered', [toolA], { count: 60, page: 1, offset: 0 }))
      }
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="discovered" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('openai/browser-agent')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('checkbox', { name: '选择 openai/browser-agent' }))
    expect(screen.getByText('1 已选')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '下一页' }))
    await waitFor(() => expect(screen.getByText('anthropic/skills')).toBeInTheDocument())
    expect(screen.getByText(/已跨页选择 1 条/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('checkbox', { name: '选择 anthropic/skills' }))
    expect(screen.getByText('2 已选')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '批量测试' }))
    fireEvent.change(screen.getByLabelText('测评人'), { target: { value: '张三' } })
    fireEvent.change(screen.getByLabelText('操作人'), { target: { value: '李四' } })
    fireEvent.click(screen.getByRole('button', { name: '提交' }))

    await waitFor(() => expect(screen.getByText('批量测评完成：2 个工具')).toBeInTheDocument())
    expect(fetchMock.mock.calls.filter((call) => call[0] === '/api/tools/evaluations/start')).toHaveLength(2)
  })

  it('batch includes evaluating tools with individual Cooper links and summaries', async () => {
    const toolA = makeTool('tool-a', 'openai/browser-agent', {
      status: 'evaluating',
      evaluation: { id: 'eval-a', evaluator: '张三', operator: '李四', startedAt: now },
    })
    const toolB = makeTool('tool-b', 'anthropic/skills', {
      status: 'evaluating',
      evaluation: { id: 'eval-b', evaluator: '张三', operator: '李四', cooperUrl: 'https://cooper.didichuxing.com/didocs/2', startedAt: now },
    })
    const fetchMock = vi.fn((url: string) => {
      if (url === '/api/tools/evaluations/cooper-url') return ok({ evaluation: { id: 'eval-a', evaluator: '张三', operator: '李四', cooperUrl: 'https://cooper.didichuxing.com/didocs/1', startedAt: now } })
      if (url === '/api/tools/evaluations/finish') return ok({ completed: true })
      if (String(url).startsWith('/api/tools/items')) return ok(toolList('evaluating', [toolA, toolB]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="evaluating" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('anthropic/skills')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('checkbox', { name: '选择 openai/browser-agent' }))
    fireEvent.click(screen.getByRole('checkbox', { name: '选择 anthropic/skills' }))
    fireEvent.click(screen.getByRole('button', { name: '批量纳入' }))
    fireEvent.change(screen.getByLabelText('操作人'), { target: { value: '王五' } })
    const cooperInputs = screen.getAllByLabelText('Cooper 文档')
    fireEvent.change(cooperInputs[0], { target: { value: 'https://cooper.didichuxing.com/didocs/1' } })
    const summaryInputs = screen.getAllByLabelText('团队使用说明')
    fireEvent.change(summaryInputs[0], { target: { value: '这是一个浏览器自动化工具，适合团队处理网页操作和验证任务。' } })
    fireEvent.change(summaryInputs[1], { target: { value: '这是一个技能仓库工具，适合团队沉淀可复用的 agent 能力。' } })
    fireEvent.click(screen.getByRole('button', { name: '提交' }))

    await waitFor(() => expect(screen.getByText('批量纳入完成：2 个工具')).toBeInTheDocument())
    expect(fetchMock.mock.calls.filter((call) => call[0] === '/api/tools/evaluations/cooper-url')).toHaveLength(1)
    expect(fetchMock.mock.calls.filter((call) => call[0] === '/api/tools/evaluations/finish')).toHaveLength(2)
  })

  it('batch edits and deletes team tools', async () => {
    const toolA = makeTool('tool-a', 'openai/browser-agent', {
      status: 'included',
      finalSummary: '团队用它处理浏览器任务。',
      includedAt: now,
      evaluation: { id: 'eval-a', evaluator: 'alice', operator: 'alice', cooperUrl: 'https://cooper.didichuxing.com/didocs/1', startedAt: now, completedAt: now, result: 'included' },
    })
    const toolB = makeTool('tool-b', 'anthropic/skills', {
      status: 'included',
      finalSummary: '团队用它沉淀技能。',
      includedAt: now,
      evaluation: { id: 'eval-b', evaluator: 'alice', operator: 'alice', cooperUrl: 'https://cooper.didichuxing.com/didocs/2', startedAt: now, completedAt: now, result: 'included' },
    })
    const fetchMock = vi.fn((url: string) => {
      if (url === '/api/tools/team/update') return ok({ updated: true })
      if (url === '/api/tools/team/delete') return ok({ deleted: true })
      if (String(url).startsWith('/api/tools/items')) return ok(toolList('included', [toolA, toolB]))
      return ok({})
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    render(<ToolsView section="team" onSection={vi.fn()} />)
    await waitFor(() => expect(screen.getByText('anthropic/skills')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('checkbox', { name: '选择 openai/browser-agent' }))
    fireEvent.click(screen.getByRole('checkbox', { name: '选择 anthropic/skills' }))
    fireEvent.click(screen.getByRole('button', { name: '批量修改' }))
    fireEvent.change(screen.getByLabelText('操作人'), { target: { value: 'bob' } })
    const summaryInputs = screen.getAllByLabelText('团队使用说明')
    fireEvent.change(summaryInputs[0], { target: { value: '更新后的浏览器自动化工具说明。' } })
    fireEvent.change(summaryInputs[1], { target: { value: '更新后的技能仓库工具说明。' } })
    fireEvent.click(screen.getByRole('button', { name: '提交' }))

    await waitFor(() => expect(screen.getByText('批量修改完成：2 个工具')).toBeInTheDocument())
    expect(fetchMock.mock.calls.filter((call) => call[0] === '/api/tools/team/update')).toHaveLength(2)

    fireEvent.click(screen.getByRole('checkbox', { name: '选择 openai/browser-agent' }))
    fireEvent.click(screen.getByRole('checkbox', { name: '选择 anthropic/skills' }))
    fireEvent.click(screen.getByRole('button', { name: '批量删除' }))
    fireEvent.change(screen.getByLabelText('操作人'), { target: { value: 'carol' } })
    fireEvent.change(screen.getByLabelText('删除原因'), { target: { value: '团队不再使用' } })
    fireEvent.click(screen.getByRole('button', { name: '提交' }))

    await waitFor(() => expect(screen.getByText('批量删除完成：2 个工具')).toBeInTheDocument())
    expect(fetchMock.mock.calls.filter((call) => call[0] === '/api/tools/team/delete')).toHaveLength(2)
  })
})
