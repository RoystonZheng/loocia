import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ToolApiError,
  addManualTool,
  deleteGitHubToken,
  deleteToolConfig,
  excludeDiscoveredTool,
  fetchToolConfigs,
  fetchToolSummaryByURLKey,
  fetchTools,
  finishToolEvaluation,
  pauseToolConfig,
  previewManualTool,
  resumeToolConfig,
  runToolConfig,
  saveGitHubToken,
  saveToolConfig,
  setToolConfigEnabled,
  startToolEvaluation,
  testGitHubToken,
  updateToolEvaluationCooperURL,
  updateToolPurposeTags,
} from './tools'

function ok(data: unknown) {
  return Promise.resolve({ ok: true, json: async () => ({ errno: 0, errmsg: 'ok', data }) })
}

function fail(status: number, errno: number, errmsg: string, className: string) {
  return Promise.resolve({ ok: false, status, json: async () => ({ errno, errmsg, data: { class: className } }) })
}

describe('tools api client', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('fetches configs with optional search and filters', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(ok({ count: 0, enabledCount: 0, disabledCount: 0, items: [] })) as unknown as typeof fetch
    const out = await fetchToolConfigs({ q: 'browser', methods: ['keyword'], triggerModes: ['manual', 'weekly'] })
    expect(out.count).toBe(0)
    const url = (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0] as string
    expect(url).toContain('/api/tools/configs?')
    expect(url).toContain('q=browser')
    expect(url).toContain('method=keyword')
    expect(url).toContain('triggerMode=manual')
    expect(url).toContain('triggerMode=weekly')
  })

  it('builds the tools list query string', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(ok({
      count: 0,
      page: 3,
      pageSize: 20,
      offset: 40,
      take: 20,
      stats: { status: 'discovered', keywordSourceCount: 0, topicSourceCount: 0, manualSourceCount: 0, linkedEvaluationCount: 0, unlinkedEvaluationCount: 0, evaluatorCount: 0, purposeTags: [], keywords: [] },
      items: [],
    })) as unknown as typeof fetch
    await fetchTools({ status: 'discovered', sort: 'stars7d', sources: ['keyword', 'manual'], purposeTags: ['浏览器操作'], keyword: 'browser agent', q: 'agent', take: 20, offset: 40, page: 3 })
    const url = (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0] as string
    const parsed = new URL(url, 'http://localhost')
    expect(url).toContain('/api/tools/items?')
    expect(url).toContain('status=discovered')
    expect(url).toContain('sort=stars7d')
    expect(url).toContain('source=keyword')
    expect(url).toContain('source=manual')
    expect(url).toContain('purposeTag=%E6%B5%8F%E8%A7%88%E5%99%A8%E6%93%8D%E4%BD%9C')
    expect(parsed.searchParams.get('keyword')).toBe('browser agent')
    expect(url).toContain('q=agent')
    expect(url).toContain('take=20')
    expect(url).toContain('offset=40')
    expect(url).toContain('page=3')
  })

  it('posts url-key summary requests', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(ok({ toolId: 'tool1', urlKey: 'url-key', summary: '中文简介', source: 'url_key:url-key', translated: true })) as unknown as typeof fetch
    const out = await fetchToolSummaryByURLKey({ toolId: 'tool1', urlKey: 'url-key' })
    expect(out.summary).toBe('中文简介')
    const call = (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(call[0]).toBe('/api/tools/summaries/url-key')
    expect(call[1]?.method).toBe('POST')
    expect(JSON.parse(String(call[1]?.body))).toEqual({ toolId: 'tool1', urlKey: 'url-key' })
  })

  it('posts token metadata and never changes the documented endpoints', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(ok({
        token: { id: 'token-1', name: '主账号', description: '日常发现', testUrl: 'https://api.github.com/rate_limit', index: 0, masked: '****abcd', last4: 'abcd', source: 'saved' },
        settings: { githubTokenCount: 1, githubTokens: [], defaultGitHubTokenCount: 0, defaultGitHubTokens: [], githubBaseUrl: '', githubTokenStrategy: 'round_robin', githubActiveTokenIndex: 0, includeDefaultGitHubTokens: false, githubMaxPages: 5, githubPerPage: 100, githubRequestIntervalMs: 2000, starSnapshotLimit: 200 },
      }))
      .mockResolvedValueOnce(ok({
        token: { id: 'token-1', name: '主账号', masked: '****abcd', last4: 'abcd', source: 'saved', ok: true, limit: 5000, remaining: 4999 },
      }))
      .mockResolvedValueOnce(ok({ deleted: true }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await saveGitHubToken({
      id: 'token-1',
      name: '主账号',
      description: '日常发现',
      testUrl: 'https://api.github.com/rate_limit',
      token: 'ghp_secret',
      actor: 'alice',
    })
    await testGitHubToken({ id: 'token-1' })
    await deleteGitHubToken('token-1', 'alice')

    expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
      '/api/tools/settings/tokens',
      '/api/tools/settings/tokens/test',
      '/api/tools/settings/tokens/delete',
    ])
    expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body))).toEqual({
      id: 'token-1',
      name: '主账号',
      description: '日常发现',
      testUrl: 'https://api.github.com/rate_limit',
      token: 'ghp_secret',
      actor: 'alice',
    })
  })

  it('posts config and state mutations to the documented endpoints', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(ok({ config: { id: 'cfg1', name: 'n', method: 'keyword', terms: ['agent'], triggerMode: 'manual', enabled: true, lastResultCount: 0, createdBy: 'a', updatedBy: 'a', createdAt: '2026-09-10T00:00:00Z', updatedAt: '2026-09-10T00:00:00Z' } }))
      .mockResolvedValueOnce(ok({ config: { id: 'cfg1', name: 'n', method: 'keyword', terms: ['agent'], triggerMode: 'manual', enabled: false, lastResultCount: 0, createdBy: 'a', updatedBy: 'b', createdAt: '2026-09-10T00:00:00Z', updatedAt: '2026-09-10T00:00:00Z' } }))
      .mockResolvedValueOnce(ok({ run: { id: 'run1', status: 'running', resultCount: 0, newCount: 0, updatedCount: 0, skippedCount: 0, pagesScanned: 0, incompleteResults: false, truncated: false, pauseRequested: false, nextQueryIndex: 0, nextPage: 1, rateLimited: false } }))
      .mockResolvedValueOnce(ok({ run: { id: 'run1', status: 'running', resultCount: 0, newCount: 0, updatedCount: 0, skippedCount: 0, pagesScanned: 0, incompleteResults: false, truncated: false, pauseRequested: true, nextQueryIndex: 0, nextPage: 1, rateLimited: false } }))
      .mockResolvedValueOnce(ok({ run: { id: 'run1', status: 'running', resultCount: 0, newCount: 0, updatedCount: 0, skippedCount: 0, pagesScanned: 0, incompleteResults: false, truncated: false, pauseRequested: false, nextQueryIndex: 0, nextPage: 1, rateLimited: false } }))
      .mockResolvedValueOnce(ok({ deleted: true }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await saveToolConfig({ name: 'n', method: 'keyword', terms: ['agent'], triggerMode: 'manual', actor: 'a' })
    await setToolConfigEnabled('cfg1', false, 'b')
    await runToolConfig('cfg1', 'b')
    await pauseToolConfig('cfg1', 'b')
    await resumeToolConfig('cfg1', 'b')
    await deleteToolConfig('cfg1', 'b')

    expect(fetchMock.mock.calls.map((c) => c[0])).toEqual([
      '/api/tools/configs',
      '/api/tools/configs/enable',
      '/api/tools/configs/run',
      '/api/tools/configs/pause',
      '/api/tools/configs/resume',
      '/api/tools/configs/delete',
    ])
    expect(fetchMock.mock.calls.every((c) => c[1]?.method === 'POST')).toBe(true)
  })

  it('posts manual add and evaluation operations', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(ok({ repository: { nodeId: 'n1', owner: 'o', repo: 'r', fullName: 'o/r', url: 'https://github.com/o/r', name: 'r', stars: 1, forks: 0, openIssues: 0, topics: [] } }))
      .mockResolvedValueOnce(ok({ tool: { id: 'tool1', name: 'r', githubNodeId: 'n1', githubOwner: 'o', githubRepo: 'r', githubFullName: 'o/r', githubUrl: 'https://github.com/o/r', currentSummarySource: 'github_description', stars: 1, forks: 0, openIssues: 0, topics: [], status: 'discovered', firstDiscoveredAt: '2026-09-10T00:00:00Z', lastDiscoveredAt: '2026-09-10T00:00:00Z', statusVersion: 1, sources: [], createdAt: '2026-09-10T00:00:00Z', updatedAt: '2026-09-10T00:00:00Z' }, created: true, duplicate: false }))
      .mockResolvedValueOnce(ok({ evaluation: { id: 'eval1', evaluator: '张三', operator: '李四', startedAt: '2026-09-10T00:00:00Z' } }))
      .mockResolvedValueOnce(ok({ evaluation: { id: 'eval1', evaluator: '张三', operator: '李四', cooperUrl: 'https://cooper.didichuxing.com/didocs/1', startedAt: '2026-09-10T00:00:00Z' } }))
      .mockResolvedValueOnce(ok({ completed: true }))
      .mockResolvedValueOnce(ok({ excluded: true }))
      .mockResolvedValueOnce(ok({ tool: { id: 'tool1', purposeTags: ['浏览器操作'] } }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await previewManualTool('https://github.com/o/r')
    await addManualTool('https://github.com/o/r', '李四')
    await startToolEvaluation({ toolId: 'tool1', evaluator: '张三', operator: '李四' })
    await updateToolEvaluationCooperURL('tool1', 'https://cooper.didichuxing.com/didocs/1', '李四')
    await finishToolEvaluation({ toolId: 'tool1', evaluationId: 'eval1', result: 'included', operator: '李四', finalSummary: '一个适合团队使用的工具' })
    await excludeDiscoveredTool('tool2', '不处理', '李四')
    await updateToolPurposeTags({ toolId: 'tool1', actor: '李四', purposeTags: ['浏览器操作'] })

    expect(fetchMock.mock.calls.map((c) => c[0])).toEqual([
      '/api/tools/manual/preview',
      '/api/tools/manual/add',
      '/api/tools/evaluations/start',
      '/api/tools/evaluations/cooper-url',
      '/api/tools/evaluations/finish',
      '/api/tools/discovered/exclude',
      '/api/tools/purpose-tags',
    ])
  })

  it('throws ToolApiError with backend class', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(fail(409, 409001, '状态已变化', 'status_conflict')) as unknown as typeof fetch
    await expect(deleteToolConfig('cfg1', 'alice')).rejects.toMatchObject({
      name: 'ToolApiError',
      errno: 409001,
      status: 409,
      className: 'status_conflict',
    } satisfies Partial<ToolApiError>)
  })
})
