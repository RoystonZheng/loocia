import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type FormEvent, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import {
  ToolApiError,
  addManualTool,
  checkGitHubSettings,
  deleteGitHubToken,
  deleteTeamTool,
  deleteToolConfig,
  excludeDiscoveredTool,
  fetchToolConfigs,
  fetchToolSettings,
  fetchToolSummaryByURLKey,
  fetchTools,
  finishToolEvaluation,
  importTeamTool,
  pauseToolConfig,
  previewManualTool,
  resumeToolConfig,
  runToolConfig,
  saveGitHubToken,
  saveToolConfig,
  saveToolSettings,
  setToolConfigEnabled,
  startToolEvaluation,
  testGitHubToken,
  updateTeamTool,
  updateToolPurposeTags,
  updateToolEvaluationCooperURL,
  type ConfigList,
  type DiscoveryConfig,
  type DiscoveryMethod,
  type GitHubRepositoryPreview,
  type GitHubTokenCheck,
  type GitHubTokenStrategy,
  type ToolItem,
  type ToolList,
  type ToolSort,
  type ToolSourceType,
  type ToolStatus,
  type ToolRuntimeSettings,
  type ToolSettingsTokenPreview,
  type TriggerMode,
} from '../api/tools'
import { formatBeijingTime } from '../format'

export type ToolsSection = 'configs' | 'accounts' | 'discovered' | 'evaluating' | 'team'

const SECTION_META: Record<ToolsSection, { title: string; sub: string; search: string }> = {
  configs: { title: '发现配置', sub: '关键词与 Topic 检索规则', search: '搜索配置或检索词' },
  accounts: { title: '抓取账号', sub: 'GitHub 工具发现的必要账号', search: '' },
  discovered: { title: '工具百宝箱', sub: 'GitHub 自动检索与团队手动添加', search: '搜索工具、仓库或发现来源' },
  evaluating: { title: '测评中', sub: '关联 Cooper 测评记录并确认是否纳入', search: '搜索工具或测评人' },
  team: { title: '团队工具', sub: '团队已验证并纳入使用的工具', search: '搜索团队工具' },
}

type SourceFilter = ToolSourceType | 'all'
type PurposeFilter = string
type ManualPurposeMode = 'ai' | 'manual'
type ConfigMethodFilter = DiscoveryMethod | 'all'
type ConfigTriggerFilter = TriggerMode | 'all'
type ConfigRunState = 'idle' | 'running' | 'pausing' | 'paused'

const SOURCE_FILTER_OPTIONS: { key: SourceFilter; label: string }[] = [
  { key: 'all', label: '全部来源' },
  { key: 'keyword', label: '关键词' },
  { key: 'topic', label: 'Topic' },
  { key: 'manual', label: '手动添加' },
]

const CONFIG_METHOD_FILTER_OPTIONS: { key: ConfigMethodFilter; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'keyword', label: '关键词' },
  { key: 'topic', label: 'Topic' },
]

const CONFIG_TRIGGER_FILTER_OPTIONS: { key: ConfigTriggerFilter; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'manual', label: '手动执行' },
  { key: 'weekly', label: '自动执行' },
]

const SORT_OPTIONS: { key: ToolSort; label: string; icon: string }[] = [
  { key: 'latest', label: '最新发现', icon: '◷' },
  { key: 'stars', label: '当前 Stars', icon: '☆' },
  { key: 'stars7d', label: '7 日 Star 增长', icon: '↗' },
]

const GITHUB_TOKEN_STRATEGIES: { value: GitHubTokenStrategy; label: string; desc: string }[] = [
  { value: 'round_robin', label: '轮流使用', desc: '每次请求按候选账号顺序轮换' },
  { value: 'fixed', label: '固定使用', desc: '始终使用选中的一个账号' },
  { value: 'failover', label: '额度不足时切换', desc: '优先使用第一个可选账号，限流后切换' },
]

const PAGE_SIZE_OPTIONS = [50, 100, 200] as const

const DEFAULT_PURPOSE_TAGS = [
  '代码开发',
  '浏览器操作',
  '深度研究',
  '知识库检索',
  'MCP 集成',
  'Agent 编排',
  '数据分析',
  '工作流自动化',
  '测试验证',
  '文档写作',
  '设计创作',
  '命令行效率',
  'API/SDK 集成',
  '提示词管理',
]

type ToolAction =
  | { kind: 'start'; tool: ToolItem }
  | { kind: 'exclude'; tool: ToolItem }
  | { kind: 'cooper'; tool: ToolItem }
  | { kind: 'include'; tool: ToolItem }
  | { kind: 'reject'; tool: ToolItem }
  | { kind: 'purpose'; tool: ToolItem }
  | { kind: 'team-edit'; tool: ToolItem }
  | { kind: 'team-delete'; tool: ToolItem }

type BatchToolAction =
  | { kind: 'batch-start'; tools: ToolItem[] }
  | { kind: 'batch-exclude'; tools: ToolItem[] }
  | { kind: 'batch-include'; tools: ToolItem[] }
  | { kind: 'batch-reject'; tools: ToolItem[] }
  | { kind: 'batch-team-edit'; tools: ToolItem[] }
  | { kind: 'batch-team-delete'; tools: ToolItem[] }

type BatchRowState = {
  toolId: string
  cooperUrl: string
  finalSummary: string
  reason: string
}

type ConfigFormState = {
  id: string
  name: string
  method: DiscoveryMethod
  termsText: string
  triggerMode: TriggerMode
  enabled: boolean
  actor: string
}

type SettingsFormState = {
  tokenId: string
  tokenName: string
  tokenDescription: string
  tokenValue: string
  tokenTestUrl: string
  githubBaseUrl: string
  useEnterpriseGitHub: boolean
  githubTokenStrategy: GitHubTokenStrategy
  githubActiveTokenIndex: string
  includeDefaultGitHubTokens: boolean
  actor: string
}

export function ToolsView({
  section,
  onSection,
}: {
  section: ToolsSection
  onSection: (section: ToolsSection) => void
}) {
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')

  if (section === 'configs') {
    return (
      <ConfigPanel
        notice={notice}
        error={error}
        onNotice={setNotice}
        onError={setError}
        onOpenSettings={() => onSection('accounts')}
      />
    )
  }
  if (section === 'accounts') {
    return (
      <AccountPanel
        notice={notice}
        error={error}
        onNotice={setNotice}
        onError={setError}
        onBack={() => onSection('configs')}
      />
    )
  }
  return <ToolListPanel section={section} notice={notice} error={error} onNotice={setNotice} onError={setError} />
}

function ToolShell({
  section,
  q,
  onQ,
  onSearch,
  actions,
  notice,
  error,
  hideSearch = false,
  children,
}: {
  section: ToolsSection
  q: string
  onQ: (v: string) => void
  onSearch: () => void
  actions?: ReactNode
  notice: string
  error: string
  hideSearch?: boolean
  children: ReactNode
}) {
  const meta = SECTION_META[section]
  return (
    <div className="tools-page">
      <header className="tools-topbar">
        {hideSearch ? (
          <div className="tools-top-spacer" />
        ) : (
          <form className="tools-top-search" role="search" onSubmit={(e) => { e.preventDefault(); onSearch() }}>
            <span aria-hidden>⌕</span>
            <input value={q} onChange={(e) => onQ(e.target.value)} placeholder={meta.search} aria-label={meta.search} />
            <button className="tools-search-submit" type="submit">搜索</button>
          </form>
        )}
        <div className="tools-top-status">
          <span className="tools-health"><i />GitHub API 正常</span>
          <span className="tools-avatar">LM</span>
        </div>
      </header>

      <main className="tools-content">
        <div className="tools-page-head">
          <div>
            <h1>{meta.title}</h1>
            <p>{meta.sub}</p>
          </div>
          {actions && <div className="tools-head-actions">{actions}</div>}
        </div>

        {notice && <p className="tools-notice">{notice}</p>}
        {error && <p className="tools-error">{error}</p>}

        {children}
      </main>
    </div>
  )
}

function ConfigPanel({
  notice,
  error,
  onNotice,
  onError,
  onOpenSettings,
}: {
  notice: string
  error: string
  onNotice: (v: string) => void
  onError: (v: string) => void
  onOpenSettings: () => void
}) {
  const [data, setData] = useState<ConfigList | null>(null)
  const [loading, setLoading] = useState(false)
  const [q, setQ] = useState('')
  const [submittedQ, setSubmittedQ] = useState('')
  const [methodFilter, setMethodFilter] = useState<ConfigMethodFilter>('all')
  const [triggerFilter, setTriggerFilter] = useState<ConfigTriggerFilter>('all')
  const [configRunOverrides, setConfigRunOverrides] = useState<Record<string, ConfigRunState>>({})
  const [formOpen, setFormOpen] = useState(false)
  const [form, setForm] = useState<ConfigFormState>(emptyConfigForm(''))

  const load = useCallback(async (silent = false) => {
    if (!silent) {
      setLoading(true)
      onError('')
    }
    try {
      const methods = methodFilter === 'all' ? undefined : [methodFilter]
      const triggerModes = triggerFilter === 'all' ? undefined : [triggerFilter]
      const next = await fetchToolConfigs({
        q: submittedQ || undefined,
        methods,
        triggerModes,
      })
      setData(next)
      setConfigRunOverrides((prev) => reconcileConfigRunOverrides(prev, next.items))
    } catch (err) {
      onError(formatToolError(err))
    } finally {
      if (!silent) setLoading(false)
    }
  }, [methodFilter, onError, submittedQ, triggerFilter])

  useEffect(() => {
    load()
  }, [load])

  const shouldPollConfigRuns = useMemo(() => {
    return (data?.items ?? []).some((cfg) => {
      const state = resolveConfigRunState(cfg, configRunOverrides)
      return state === 'running' || state === 'pausing'
    })
  }, [configRunOverrides, data?.items])

  useEffect(() => {
    if (!shouldPollConfigRuns) return
    const timer = window.setInterval(() => {
      void load(true)
    }, 3000)
    return () => window.clearInterval(timer)
  }, [load, shouldPollConfigRuns])

  async function submit(e: FormEvent) {
    e.preventDefault()
    onError('')
    const existingConfig = form.id ? data?.items.find((cfg) => cfg.id === form.id) : undefined
    const actor = actorForConfigAction(existingConfig)
    if (!actor) {
      onError('请先填写操作人')
      return
    }
    try {
      const saved = await saveToolConfig({
        id: form.id || undefined,
        name: form.name,
        method: form.method,
        terms: splitTerms(form.termsText),
        triggerMode: form.triggerMode,
        enabled: form.enabled,
        actor,
      })
      onNotice(form.id ? '配置已更新' : '配置已创建')
      setForm((prev) => ({
        ...prev,
        id: saved.id,
        name: saved.name,
        termsText: saved.terms.join('\n'),
        method: saved.method,
        triggerMode: saved.triggerMode,
        enabled: saved.enabled,
      }))
      setFormOpen(false)
      await load()
    } catch (err) {
      onError(formatToolError(err))
    }
  }

  async function toggleConfig(cfg: DiscoveryConfig) {
    onError('')
    const actor = actorForConfigAction(cfg)
    if (!actor) {
      onError('请先填写操作人')
      return
    }
    try {
      await setToolConfigEnabled(cfg.id, !cfg.enabled, actor)
      onNotice(cfg.enabled ? '配置已停用' : '配置已启用')
      await load()
    } catch (err) {
      onError(formatToolError(err))
    }
  }

  async function runConfig(cfg: DiscoveryConfig) {
    onError('')
    const actor = actorForConfigAction(cfg)
    if (!actor) {
      onError('请先填写操作人')
      return
    }
    const state = resolveConfigRunState(cfg, configRunOverrides)
    if (state === 'pausing') {
      onNotice('正在暂停，请等待当前请求完成')
      return
    }
    try {
      if (state === 'running') {
        setConfigRunOverrides((prev) => ({ ...prev, [cfg.id]: 'pausing' }))
        const run = await pauseToolConfig(cfg.id, actor)
        onNotice(run.status === 'paused' ? '配置已暂停' : '已请求暂停，当前请求完成后会暂停')
        await load(true)
        return
      }
      if (state === 'paused') {
        setConfigRunOverrides((prev) => ({ ...prev, [cfg.id]: 'running' }))
        await resumeToolConfig(cfg.id, actor)
        onNotice('已继续执行')
        await load(true)
        return
      }
      setConfigRunOverrides((prev) => ({ ...prev, [cfg.id]: 'running' }))
      await runToolConfig(cfg.id, actor)
      onNotice('已开始执行，刷新页面不会中断')
      await load(true)
    } catch (err) {
      setConfigRunOverrides((prev) => {
        const next = { ...prev }
        delete next[cfg.id]
        return next
      })
      if (err instanceof ToolApiError && err.className === 'status_conflict') {
        onNotice('当前配置状态已变化，正在刷新')
        await load(true)
      } else {
        onError(formatToolError(err))
      }
    }
  }

  async function removeConfig(cfg: DiscoveryConfig) {
    if (!window.confirm(`删除配置「${cfg.name}」？历史运行和工具百宝箱条目会保留。`)) return
    onError('')
    const actor = actorForConfigAction(cfg)
    if (!actor) {
      onError('请先填写操作人')
      return
    }
    try {
      await deleteToolConfig(cfg.id, actor)
      onNotice('配置已删除')
      if (form.id === cfg.id) setForm(emptyConfigForm(form.actor))
      await load()
    } catch (err) {
      onError(formatToolError(err))
    }
  }

  function openNewConfig() {
    setForm(emptyConfigForm(form.actor))
    setFormOpen(true)
  }

  function editConfig(cfg: DiscoveryConfig) {
    setForm({
      id: cfg.id,
      name: cfg.name,
      method: cfg.method,
      termsText: cfg.terms.join('\n'),
      triggerMode: cfg.triggerMode,
      enabled: cfg.enabled,
      actor: form.actor.trim() || cfg.updatedBy || cfg.createdBy,
    })
    onNotice('正在编辑配置')
    setFormOpen(true)
  }

  function actorForConfigAction(cfg?: DiscoveryConfig): string {
    return form.actor.trim() || cfg?.updatedBy || cfg?.createdBy || ''
  }

  function runStateForConfig(cfg: DiscoveryConfig): ConfigRunState {
    return resolveConfigRunState(cfg, configRunOverrides)
  }

  return (
    <ToolShell
      section="configs"
      q={q}
      onQ={setQ}
      onSearch={() => setSubmittedQ(q.trim())}
      notice={notice}
      error={error}
      actions={(
        <>
          <button type="button" className="tools-btn" onClick={onOpenSettings}>配置设置</button>
          <button type="button" className="tools-btn primary" onClick={openNewConfig}><span aria-hidden>＋</span>新增配置</button>
        </>
      )}
    >
      <SummaryStrip aside="仅使用 GitHub Repository Search API">
        <SummaryMetric value={data?.count ?? 0} label="条配置" />
        <SummaryMetric value={data?.enabledCount ?? 0} label="已启用" />
        <SummaryMetric value={data?.disabledCount ?? 0} label="已停用" />
      </SummaryStrip>
      <ConfigFilterBar
        methodFilter={methodFilter}
        triggerFilter={triggerFilter}
        onMethodFilter={setMethodFilter}
        onTriggerFilter={setTriggerFilter}
      />

      {loading && <LoadingBlock />}
      {!loading && data?.items.length === 0 && <EmptyBlock text="暂无配置" />}
      <div className="tools-config-list">
        {data?.items.map((cfg) => (
          <ConfigCard
            key={cfg.id}
            cfg={cfg}
            runState={runStateForConfig(cfg)}
            onRun={runConfig}
            onEdit={editConfig}
            onToggle={toggleConfig}
            onDelete={removeConfig}
          />
        ))}
      </div>

      {formOpen && (
        <ToolModal title={form.id ? '编辑发现配置' : '新增发现配置'} onClose={() => setFormOpen(false)}>
          <form className="tools-modal-form" onSubmit={submit}>
            <label>
              <span>配置名称</span>
              <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="如：浏览器操作工具发现" />
            </label>
            <div className="tools-form-grid">
              <label>
                <span>发现方式</span>
                <select value={form.method} onChange={(e) => setForm({ ...form, method: e.target.value as DiscoveryMethod })}>
                  <option value="keyword">关键词</option>
                  <option value="topic">Topic</option>
                </select>
              </label>
              <label>
                <span>触发方式</span>
                <select value={form.triggerMode} onChange={(e) => setForm({ ...form, triggerMode: e.target.value as TriggerMode })}>
                  <option value="manual">手动执行</option>
                  <option value="weekly">每周执行</option>
                </select>
              </label>
            </div>
            <label>
              <span>{form.method === 'keyword' ? '检索词' : 'Topics'}</span>
              <textarea
                value={form.termsText}
                onChange={(e) => setForm({ ...form, termsText: e.target.value })}
                placeholder={form.method === 'keyword' ? 'browser agent\ncomputer use' : 'browser-agent\nbrowser-automation'}
              />
            </label>
            <label>
              <span>操作人</span>
              <input value={form.actor} onChange={(e) => setForm({ ...form, actor: e.target.value })} placeholder="手动填写" />
            </label>
            <label className="tools-check">
              <input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} />
              <span>启用配置</span>
            </label>
            <div className="tools-modal-actions">
              <button type="button" className="tools-btn" onClick={() => setFormOpen(false)}>取消</button>
              <button className="tools-btn primary" type="submit">{form.id ? '保存配置' : '创建配置'}</button>
            </div>
          </form>
        </ToolModal>
      )}
    </ToolShell>
  )
}

function AccountPanel({
  notice,
  error,
  onNotice,
  onError,
  onBack,
}: {
  notice: string
  error: string
  onNotice: (v: string) => void
  onError: (v: string) => void
  onBack: () => void
}) {
  const [settings, setSettings] = useState<ToolRuntimeSettings | null>(null)
  const [form, setForm] = useState<SettingsFormState>(emptySettingsForm('', null))
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [checking, setChecking] = useState(false)
  const [checks, setChecks] = useState<GitHubTokenCheck[]>([])

  const load = useCallback(async () => {
    setLoading(true)
    onError('')
    try {
      const next = normalizeSettings(await fetchToolSettings())
      setSettings(next)
      setForm(emptySettingsForm(next.updatedBy || '', next))
    } catch (err) {
      onError(formatToolError(err))
    } finally {
      setLoading(false)
    }
  }, [onError])

  useEffect(() => {
    load()
  }, [load])

  const savedTokens = settings?.githubTokens ?? []
  const defaultTokens = settings?.defaultGitHubTokens ?? []
  const candidates = [...(form.includeDefaultGitHubTokens ? defaultTokens : []), ...savedTokens]
  const fixedCandidates = candidates

  async function submit(e: FormEvent) {
    e.preventDefault()
    onError('')
    const actor = form.actor.trim() || settings?.updatedBy || 'system'
    const activeIndex = parseActiveTokenIndex(form.githubActiveTokenIndex)
    if (form.githubTokenStrategy === 'fixed' && fixedCandidates.length === 0) {
      onError('请先配置至少一个 GitHub Token')
      return
    }
    if (form.githubTokenStrategy === 'fixed' && activeIndex >= fixedCandidates.length) {
      onError('固定 Token 已超出当前候选范围')
      return
    }
    try {
      setSaving(true)
      const saved = await saveToolSettings({
        githubBaseUrl: form.useEnterpriseGitHub ? form.githubBaseUrl.trim() : '',
        githubTokenStrategy: form.githubTokenStrategy,
        githubActiveTokenIndex: activeIndex,
        includeDefaultGitHubTokens: form.includeDefaultGitHubTokens,
        actor,
      })
      const normalized = normalizeSettings(saved)
      setSettings(normalized)
      setForm(emptySettingsForm(actor, normalized))
      setChecks([])
      onNotice('抓取账号已保存')
    } catch (err) {
      onError(formatToolError(err))
    } finally {
      setSaving(false)
    }
  }

  function resetTokenForm(nextSettings = settings) {
    setForm((current) => ({
      ...emptySettingsForm(current.actor || nextSettings?.updatedBy || '', nextSettings),
      githubBaseUrl: current.githubBaseUrl,
      useEnterpriseGitHub: current.useEnterpriseGitHub,
      githubTokenStrategy: current.githubTokenStrategy,
      githubActiveTokenIndex: current.githubActiveTokenIndex,
      includeDefaultGitHubTokens: current.includeDefaultGitHubTokens,
      actor: current.actor || nextSettings?.updatedBy || '',
    }))
  }

  function editToken(token: ToolSettingsTokenPreview) {
    setForm((current) => ({
      ...current,
      tokenId: token.id,
      tokenName: token.name,
      tokenDescription: token.description ?? '',
      tokenValue: '',
      tokenTestUrl: token.testUrl ?? '',
    }))
    onError('')
    onNotice(`正在编辑 ${token.name}`)
  }

  async function saveToken() {
    onError('')
    const name = form.tokenName.trim()
    if (!name) {
      onError('请输入 Token 名称')
      return
    }
    if (!form.tokenId && !form.tokenValue.trim()) {
      onError('请输入 Token 值')
      return
    }
    const actor = form.actor.trim() || settings?.updatedBy || 'system'
    try {
      setSaving(true)
      const result = await saveGitHubToken({
        id: form.tokenId || undefined,
        name,
        description: form.tokenDescription.trim() || undefined,
        testUrl: form.tokenTestUrl.trim() || undefined,
        token: form.tokenValue.trim() || undefined,
        actor,
      })
      const normalized = normalizeSettings(result.settings)
      setSettings(normalized)
      setForm(emptySettingsForm(actor, normalized))
      setChecks([])
      onNotice(`${form.tokenId ? 'Token 已更新' : 'Token 已保存'}：${result.token.name}`)
    } catch (err) {
      onError(formatToolError(err))
    } finally {
      setSaving(false)
    }
  }

  async function removeToken(token: ToolSettingsTokenPreview) {
    if (!window.confirm(`确定删除 Token「${token.name}」吗？`)) return
    onError('')
    const actor = form.actor.trim() || settings?.updatedBy || 'system'
    try {
      setSaving(true)
      await deleteGitHubToken(token.id, actor)
      const next = normalizeSettings(await fetchToolSettings())
      setSettings(next)
      resetTokenForm(next)
      setChecks([])
      onNotice(`Token 已删除：${token.name}`)
    } catch (err) {
      onError(formatToolError(err))
    } finally {
      setSaving(false)
    }
  }

  async function testTokenDraft() {
    onError('')
    if (!form.tokenId && !form.tokenValue.trim()) {
      onError('请输入 Token 值后再测试')
      return
    }
    try {
      setChecking(true)
      const result = await testGitHubToken({
        id: form.tokenId || undefined,
        name: form.tokenName.trim() || undefined,
        description: form.tokenDescription.trim() || undefined,
        testUrl: form.tokenTestUrl.trim() || undefined,
        token: form.tokenValue.trim() || undefined,
        githubBaseUrl: form.useEnterpriseGitHub ? form.githubBaseUrl.trim() : '',
      })
      setChecks([result])
      onNotice(result.ok ? `Token 可用，剩余额度 ${result.remaining ?? 0}/${result.limit ?? 0}` : result.error || 'Token 不可用')
    } catch (err) {
      onError(formatToolError(err))
    } finally {
      setChecking(false)
    }
  }

  async function testSavedToken(token: ToolSettingsTokenPreview) {
    onError('')
    try {
      setChecking(true)
      const result = await testGitHubToken({
        id: token.id,
        githubBaseUrl: form.useEnterpriseGitHub ? form.githubBaseUrl.trim() : '',
      })
      setChecks([result])
      onNotice(result.ok ? `Token 可用，剩余额度 ${result.remaining ?? 0}/${result.limit ?? 0}` : result.error || 'Token 不可用')
    } catch (err) {
      onError(formatToolError(err))
    } finally {
      setChecking(false)
    }
  }

  async function testConnection() {
    onError('')
    setChecking(true)
    try {
      const result = await checkGitHubSettings({
        githubBaseUrl: form.useEnterpriseGitHub ? form.githubBaseUrl.trim() : '',
        includeDefaultGitHubTokens: form.includeDefaultGitHubTokens,
      })
      setChecks(result)
      onNotice(result.some((item) => item.ok) ? 'GitHub Token 校验完成' : '没有可用的 GitHub Token')
    } catch (err) {
      onError(formatToolError(err))
    } finally {
      setChecking(false)
    }
  }

  return (
    <ToolShell
      section="accounts"
      q=""
      onQ={() => {}}
      onSearch={() => {}}
      notice={notice}
      error={error}
      hideSearch
      actions={<button type="button" className="tools-btn" onClick={onBack}>返回发现配置</button>}
    >
      <SummaryStrip aside="仅影响 GitHub 工具发现；资讯 RSS 不使用这里的账号">
        <SummaryMetric value={(settings?.defaultGitHubTokenCount ?? 0) + (settings?.githubTokenCount ?? 0)} label="候选 Token" />
        <SummaryMetric value={tokenStrategyLabel(form.githubTokenStrategy)} label="使用策略" />
        <SummaryMetric value={settings?.updatedBy || '未保存'} label="最近操作" />
      </SummaryStrip>

      {loading ? <LoadingBlock /> : (
        <form className="tools-account-panel" onSubmit={submit}>
          <section className="tools-account-section">
            <div className="tools-account-head">
              <div>
                <h2>GitHub Token</h2>
                <p>工具发现通过 GitHub HTTPS API 抓取仓库，只需要这里的 Token。</p>
              </div>
              <div className="tools-account-actions">
                <button type="button" className="tools-btn" onClick={testConnection} disabled={checking}>
                  {checking ? '测试中...' : '测试连接'}
                </button>
                <button className="tools-btn primary" type="submit" disabled={saving}>
                  {saving ? '保存中...' : '保存策略'}
                </button>
              </div>
            </div>

            {form.includeDefaultGitHubTokens && defaultTokens.length > 0 && (
              <div className="tools-token-list" aria-label="环境 Token">
                {defaultTokens.map((token, index) => (
                  <TokenPill key={`default-${index}`} token={token} label="环境" />
                ))}
              </div>
            )}

            <div className="tools-token-grid" aria-label="已保存 Token">
              {savedTokens.map((token) => (
                <article className="tools-token-card" key={token.id}>
                  <div className="tools-token-main">
                    <div className="tools-token-title">
                      <strong>{token.name}</strong>
                      <code>{token.masked}</code>
                    </div>
                    {token.description && <p>{token.description}</p>}
                    {token.testUrl ? (
                      <a href={token.testUrl} target="_blank" rel="noreferrer">
                        测试链接
                      </a>
                    ) : (
                      <span className="tools-muted">未设置测试链接</span>
                    )}
                  </div>
                  <div className="tools-token-card-actions">
                    <button type="button" className="tools-btn" onClick={() => testSavedToken(token)} disabled={checking || saving}>测试</button>
                    <button type="button" className="tools-btn" onClick={() => editToken(token)} disabled={saving}>编辑</button>
                    <button type="button" className="tools-btn danger" onClick={() => removeToken(token)} disabled={saving}>删除</button>
                  </div>
                </article>
              ))}
              {savedTokens.length === 0 && <span className="tools-muted">还没有已保存 Token，请在下方添加。</span>}
            </div>

            <div className="tools-token-editor">
              <div className="tools-account-head compact">
                <div>
                  <h2>{form.tokenId ? '编辑 Token' : '添加 Token'}</h2>
                  <p>名称、描述和测试链接会保存为账号信息，Token 值仅用于鉴权。</p>
                </div>
                {form.tokenId && (
                  <button type="button" className="tools-btn" onClick={() => resetTokenForm()} disabled={saving}>
                    取消编辑
                  </button>
                )}
              </div>
              <div className="tools-token-form-grid">
                <label>
                  <span>Token 名称</span>
                  <input value={form.tokenName} onChange={(e) => setForm({ ...form, tokenName: e.target.value })} placeholder="例如：主账号" />
                </label>
                <label>
                  <span>Token 描述</span>
                  <input value={form.tokenDescription} onChange={(e) => setForm({ ...form, tokenDescription: e.target.value })} placeholder="例如：用于日常工具发现" />
                </label>
                <label className="tools-token-form-wide">
                  <span>Token 值</span>
                  <input
                    type="password"
                    value={form.tokenValue}
                    onChange={(e) => setForm({ ...form, tokenValue: e.target.value })}
                    placeholder={form.tokenId ? '留空表示保持原 Token' : '粘贴 GitHub personal access token'}
                    autoComplete="new-password"
                  />
                </label>
                <label className="tools-token-form-wide">
                  <span>测试链接</span>
                  <input value={form.tokenTestUrl} onChange={(e) => setForm({ ...form, tokenTestUrl: e.target.value })} placeholder="https://api.github.com/rate_limit" />
                </label>
              </div>
              <div className="tools-account-actions tools-account-subactions">
                <button type="button" className="tools-btn" onClick={testTokenDraft} disabled={checking || saving}>
                  {checking ? '测试中...' : '测试 Token'}
                </button>
                <button type="button" className="tools-btn primary" onClick={saveToken} disabled={saving || checking}>
                  {saving ? '保存中...' : '保存 Token'}
                </button>
              </div>
            </div>

            {defaultTokens.length > 0 && (
              <label className="tools-check">
                <input
                  type="checkbox"
                  checked={form.includeDefaultGitHubTokens}
                  onChange={(e) => setForm({ ...form, includeDefaultGitHubTokens: e.target.checked })}
                />
                <span>把环境里的默认初始 Token 加入候选账号</span>
              </label>
            )}
          </section>

          <section className="tools-account-section">
            <div className="tools-account-head compact">
              <div>
                <h2>使用策略</h2>
              </div>
            </div>
            <div className="tools-strategy-grid">
              {GITHUB_TOKEN_STRATEGIES.map((item) => (
                <label key={item.value} className={`tools-strategy-card${form.githubTokenStrategy === item.value ? ' selected' : ''}`}>
                  <input
                    type="radio"
                    name="githubTokenStrategy"
                    value={item.value}
                    aria-label={item.label}
                    checked={form.githubTokenStrategy === item.value}
                    onChange={() => setForm({ ...form, githubTokenStrategy: item.value })}
                  />
                  <strong>{item.label}</strong>
                  <span>{item.desc}</span>
                </label>
              ))}
            </div>
            {form.githubTokenStrategy === 'fixed' && (
              <label>
                <span>固定使用</span>
                <select value={form.githubActiveTokenIndex} onChange={(e) => setForm({ ...form, githubActiveTokenIndex: e.target.value })}>
                  {fixedCandidates.map((token, index) => (
                    <option key={`${token.source}-${index}`} value={String(index)}>
                      {index + 1}. {token.source === 'default' ? '默认' : '已保存'} {token.masked}
                    </option>
                  ))}
                </select>
              </label>
            )}
          </section>

          <section className="tools-account-section">
            <label className="tools-check">
              <input
                type="checkbox"
                checked={form.useEnterpriseGitHub}
                onChange={(e) => setForm({ ...form, useEnterpriseGitHub: e.target.checked, githubBaseUrl: e.target.checked ? form.githubBaseUrl : '' })}
              />
              <span>使用企业 GitHub API 地址</span>
            </label>
            {form.useEnterpriseGitHub && (
              <label>
                <span>GitHub API Base URL</span>
                <input value={form.githubBaseUrl} onChange={(e) => setForm({ ...form, githubBaseUrl: e.target.value })} placeholder="https://github.example/api/v3" />
              </label>
            )}
            <label>
              <span>操作人</span>
              <input value={form.actor} onChange={(e) => setForm({ ...form, actor: e.target.value })} placeholder="手动填写" />
            </label>
          </section>

          {checks.length > 0 && (
            <section className="tools-account-section">
              <div className="tools-account-head compact">
                <div>
                  <h2>连接测试结果</h2>
                </div>
              </div>
              <div className="tools-check-results">
                {checks.map((item) => (
                  <div key={`${item.source}-${item.index}`} className={`tools-check-result ${item.ok ? 'ok' : 'bad'}`}>
                    <strong>{item.name || (item.source === 'default' ? '环境 Token' : 'Token')} {item.masked}</strong>
                    <span>{item.ok ? `剩余额度 ${item.remaining ?? 0}/${item.limit ?? 0}` : item.error || '不可用'}</span>
                  </div>
                ))}
              </div>
            </section>
          )}
        </form>
      )}
    </ToolShell>
  )
}

function TokenPill({ token, label }: { token: ToolSettingsTokenPreview; label: string }) {
  return (
    <span className="tools-token-pill">
      <i>{label}</i>
      <strong>{token.name}</strong>
      <strong>{token.masked}</strong>
    </span>
  )
}

function ConfigCard({
  cfg,
  runState,
  onRun,
  onEdit,
  onToggle,
  onDelete,
}: {
  cfg: DiscoveryConfig
  runState: ConfigRunState
  onRun: (cfg: DiscoveryConfig) => void
  onEdit: (cfg: DiscoveryConfig) => void
  onToggle: (cfg: DiscoveryConfig) => void
  onDelete: (cfg: DiscoveryConfig) => void
}) {
  const action = configRunButtonMeta(runState)
  const progressText = configRunProgressText(cfg, runState)
  return (
    <article className={`tools-config-card ${cfg.enabled ? 'enabled' : 'disabled'}`}>
      <div className={`tools-config-type ${cfg.method}`}>
        <span aria-hidden>{cfg.method === 'keyword' ? '⌕' : '⌁'}</span>
      </div>
      <div className="tools-config-main">
        <h3>{cfg.name}</h3>
        <p>
          {cfg.method === 'keyword' ? '关键词检索' : 'Topic 检索'}
          <span className={`tools-mini-status ${cfg.enabled ? 'ok' : ''}`}>{cfg.enabled ? '已启用' : '已停用'}</span>
        </p>
      </div>
      <div className="tools-config-terms">
        {cfg.terms.map((term) => <span key={term}>{term}</span>)}
      </div>
      <div className="tools-config-schedule">
        <strong>{cfg.triggerMode === 'weekly' ? '每周执行' : '仅手动'}</strong>
        <span>上次执行 {formatShortDateTime(cfg.lastRunAt) || '暂无'}</span>
        {progressText ? <em>{progressText}</em> : cfg.lastFailureReason && <em>{configRunReasonLabel(cfg.lastFailureReason)}：{formatConfigRunReason(cfg.lastFailureReason)}</em>}
      </div>
      <div className="tools-icon-actions">
        <button type="button" aria-label={action.label} title={action.title} onClick={() => onRun(cfg)}>
          <span aria-hidden>{action.icon}</span>
        </button>
        <button type="button" aria-label="编辑" title="编辑" onClick={() => onEdit(cfg)}><span aria-hidden>✎</span></button>
        <button type="button" aria-label={cfg.enabled ? '停用' : '启用'} title={cfg.enabled ? '停用' : '启用'} onClick={() => onToggle(cfg)}>
          <span aria-hidden>{cfg.enabled ? '⏻' : '↻'}</span>
        </button>
        <button type="button" className="danger" aria-label="删除" title="删除" onClick={() => onDelete(cfg)}><span aria-hidden>⌫</span></button>
      </div>
    </article>
  )
}

function ToolListPanel({
  section,
  notice,
  error,
  onNotice,
  onError,
}: {
  section: Exclude<ToolsSection, 'configs'>
  notice: string
  error: string
  onNotice: (v: string) => void
  onError: (v: string) => void
}) {
  const status = statusForSection(section)
  const [data, setData] = useState<ToolList | null>(null)
  const [loading, setLoading] = useState(false)
  const [q, setQ] = useState('')
  const [submittedQ, setSubmittedQ] = useState('')
  const [sort, setSort] = useState<ToolSort>('latest')
  const [sourceFilter, setSourceFilter] = useState<SourceFilter>('all')
  const [purposeFilter, setPurposeFilter] = useState<PurposeFilter>('all')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(50)
  const [summaryOverrides, setSummaryOverrides] = useState<Record<string, string>>({})
  const summaryRequestIdsRef = useRef<Set<string>>(new Set())
  const [action, setAction] = useState<ToolAction | null>(null)
  const [batchAction, setBatchAction] = useState<BatchToolAction | null>(null)
  const [selectedToolMap, setSelectedToolMap] = useState<Record<string, ToolItem>>({})
  const [manualOpen, setManualOpen] = useState(false)
  const [manualRepoURL, setManualRepoURL] = useState('')
  const [manualActor, setManualActor] = useState('')
  const [manualCooperUrl, setManualCooperUrl] = useState('')
  const [manualSummary, setManualSummary] = useState('')
  const [manualPurposeMode, setManualPurposeMode] = useState<ManualPurposeMode>('ai')
  const [manualPurposeTags, setManualPurposeTags] = useState<string[]>([])
  const [manualError, setManualError] = useState('')
  const [preview, setPreview] = useState<GitHubRepositoryPreview | null>(null)
  const [manualLoading, setManualLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    onError('')
    try {
      const sources = sourceFilter === 'all' ? undefined : [sourceFilter]
      const purposeTags = purposeFilter === 'all' ? undefined : [purposeFilter]
      const next = await fetchTools({
        status,
        sort,
        sources,
        purposeTags,
        q: submittedQ || undefined,
        take: pageSize,
        offset: (page - 1) * pageSize,
      })
      setData(next)
      if (next.items.length === 0 && next.count > 0 && page > 1) {
        setPage(Math.max(1, Math.ceil(next.count / pageSize)))
      }
      setSelectedToolMap((prev) => {
        let changed = false
        const updated = { ...prev }
        for (const tool of next.items) {
          if (updated[tool.id]) {
            updated[tool.id] = tool
            changed = true
          }
        }
        return changed ? updated : prev
      })
    } catch (err) {
      onError(formatToolError(err))
    } finally {
      setLoading(false)
    }
  }, [onError, page, pageSize, purposeFilter, sort, sourceFilter, status, submittedQ])

  useEffect(() => {
    setAction(null)
    setBatchAction(null)
    setSelectedToolMap({})
    setPage(1)
    setPreview(null)
    setManualOpen(false)
    setSourceFilter('all')
    setPurposeFilter('all')
    setManualCooperUrl('')
    setManualSummary('')
    setManualPurposeMode('ai')
    setManualPurposeTags([])
    setManualError('')
    setSummaryOverrides({})
    summaryRequestIdsRef.current.clear()
  }, [section])

  useEffect(() => {
    if (!manualError) return
    const timer = window.setTimeout(() => setManualError(''), 3000)
    return () => window.clearTimeout(timer)
  }, [manualError])

  const visibleTools = useMemo(() => data?.items ?? [], [data?.items])
  const selectedTools = useMemo(() => Object.values(selectedToolMap), [selectedToolMap])
  const selectedIds = useMemo(() => Object.keys(selectedToolMap), [selectedToolMap])

  function resetListPosition() {
    setPage(1)
    setSelectedToolMap({})
  }

  function toggleSelected(toolID: string, checked: boolean) {
    const tool = visibleTools.find((item) => item.id === toolID)
    setSelectedToolMap((prev) => {
      if (checked && tool) return { ...prev, [toolID]: tool }
      const next = { ...prev }
      delete next[toolID]
      return next
    })
  }

  function toggleAllVisible(checked: boolean) {
    setSelectedToolMap((prev) => {
      if (checked) {
        const next = { ...prev }
        for (const tool of visibleTools) next[tool.id] = tool
        return next
      }
      const removeIDs = new Set(visibleTools.map((tool) => tool.id))
      const next: Record<string, ToolItem> = {}
      for (const [id, tool] of Object.entries(prev)) {
        if (!removeIDs.has(id)) next[id] = tool
      }
      return next
    })
  }

  useEffect(() => {
    load()
  }, [load])

  const ensureChineseSummary = useCallback(async (tool: ToolItem) => {
    const current = summaryOverrides[tool.id] || tool.currentSummary || tool.description || ''
    if (!tool.summaryKey || hasCJKText(current) || summaryRequestIdsRef.current.has(tool.id)) return
    summaryRequestIdsRef.current.add(tool.id)
    try {
      const res = await fetchToolSummaryByURLKey({ toolId: tool.id, urlKey: tool.summaryKey })
      if (res.summary.trim()) {
        setSummaryOverrides((prev) => ({ ...prev, [tool.id]: res.summary.trim() }))
      }
    } catch {
      summaryRequestIdsRef.current.delete(tool.id)
      // Tooltip still shows the original GitHub description if summary generation is unavailable.
    }
  }, [summaryOverrides])

  useEffect(() => {
    const missing = visibleTools.filter((tool) => {
      const current = summaryOverrides[tool.id] || tool.currentSummary || tool.description || ''
      return Boolean(tool.summaryKey && !hasCJKText(current) && !summaryRequestIdsRef.current.has(tool.id))
    })
    if (!missing.length) return
    let index = 0
    let cancelled = false
    const workerCount = Math.min(8, missing.length)
    const runWorker = async () => {
      while (!cancelled && index < missing.length) {
        const tool = missing[index]
        index += 1
        await ensureChineseSummary(tool)
      }
    }
    for (let i = 0; i < workerCount; i += 1) {
      void runWorker()
    }
    return () => {
      cancelled = true
    }
  }, [ensureChineseSummary, summaryOverrides, visibleTools])

  async function doPreview(e: FormEvent) {
    e.preventDefault()
    setManualLoading(true)
    onError('')
    try {
      const nextPreview = await previewManualTool(manualRepoURL)
      setPreview(nextPreview)
      setManualPurposeTags(nextPreview.purposeTags ?? [])
      onNotice('仓库预览已同步')
    } catch (err) {
      setPreview(null)
      onError(formatToolError(err))
    } finally {
      setManualLoading(false)
    }
  }

  async function doManualAdd() {
    if (!manualActor.trim()) {
      setManualError('请填写操作人')
      return
    }
    setManualLoading(true)
    onError('')
    try {
      const manualTags = manualPurposeMode === 'manual' ? manualPurposeTags : undefined
      const res = section === 'team'
        ? await importTeamTool({ repoUrl: manualRepoURL, operator: manualActor, cooperUrl: manualCooperUrl, finalSummary: manualSummary, purposeTags: manualTags })
        : await addManualTool(manualRepoURL, manualActor, manualTags)
      onNotice(section === 'team'
        ? (res.duplicate ? '团队工具已更新' : '工具已加入团队工具')
        : (res.duplicate ? '仓库已存在，已合并手动来源' : '仓库已加入已发现列表'))
      setManualRepoURL('')
      setManualCooperUrl('')
      setManualSummary('')
      setManualPurposeMode('ai')
      setManualPurposeTags([])
      setPreview(null)
      setManualOpen(false)
      await load()
    } catch (err) {
      onError(formatToolError(err))
    } finally {
      setManualLoading(false)
    }
  }

  const stats = data?.stats
  const purposeOptions = useMemo(() => mergePurposeOptions(stats?.purposeTags ?? [], visibleTools), [stats?.purposeTags, visibleTools])
  const allPurposeTags = useMemo(
    () => mergeTagOptions(
      DEFAULT_PURPOSE_TAGS,
      stats?.purposeTags ?? [],
      visibleTools.flatMap((tool) => tool.purposeTags ?? []),
      selectedTools.flatMap((tool) => tool.purposeTags ?? []),
      preview?.purposeTags ?? [],
    ),
    [preview?.purposeTags, selectedTools, stats?.purposeTags, visibleTools],
  )
  const summary = useMemo(() => {
    if (section === 'discovered') {
      return {
        aside: `最近同步 ${formatShortDateTime(stats?.latestUpdatedAt) || '暂无'}`,
        metrics: [
          ['待处理', data?.count ?? 0],
          ['关键词命中', stats?.keywordSourceCount ?? 0],
          ['Topic 命中', stats?.topicSourceCount ?? 0],
          ['手动添加', stats?.manualSourceCount ?? 0],
        ] as const,
      }
    }
    if (section === 'evaluating') {
      return {
        aside: '',
        metrics: [
          ['测评中', data?.count ?? 0],
          ['已关联文档', stats?.linkedEvaluationCount ?? 0],
          ['待关联文档', stats?.unlinkedEvaluationCount ?? 0],
        ] as const,
      }
    }
    return {
      aside: `更新于 ${formatShortDateTime(stats?.latestUpdatedAt) || '暂无'}`,
      metrics: [
        ['已纳入', data?.count ?? 0],
        ['名测评人', stats?.evaluatorCount ?? 0],
      ] as const,
    }
  }, [data?.count, section, stats])

  return (
    <ToolShell
      section={section}
      q={q}
      onQ={setQ}
      onSearch={() => {
        setSubmittedQ(q.trim())
        resetListPosition()
      }}
      notice={notice}
      error={error}
      actions={(section === 'discovered' || section === 'team') && (
        <button type="button" className="tools-btn primary" onClick={() => setManualOpen(true)}>
          <span aria-hidden>＋</span>{section === 'team' ? '手动导入' : '手动添加'}
        </button>
      )}
    >
      <SummaryStrip aside={summary.aside}>
        {summary.metrics.map(([label, value]) => <SummaryMetric key={label} value={value} label={label} />)}
      </SummaryStrip>

      <section className="tools-table-card">
        <div className="tools-table-toolbar">
          <div className="tools-segments" aria-label="排序方式">
            {SORT_OPTIONS.map((opt) => (
              <button
                key={opt.key}
                type="button"
                className={sort === opt.key ? 'active' : ''}
                onClick={() => {
                  setSort(opt.key)
                  resetListPosition()
                }}
              >
                <span aria-hidden>{opt.icon}</span>{opt.label}
              </button>
            ))}
          </div>
          <SelectFilter
            label="来源"
            options={SOURCE_FILTER_OPTIONS}
            value={sourceFilter}
            onChange={(next) => {
              setSourceFilter(next)
              resetListPosition()
            }}
          />
          <SelectFilter
            label="用途"
            options={purposeOptions}
            value={purposeFilter}
            onChange={(next) => {
              setPurposeFilter(next)
              resetListPosition()
            }}
          />
          <BatchActionBar
            section={section}
            selectedTools={selectedTools}
            onBatchAction={setBatchAction}
            onClear={() => setSelectedToolMap({})}
          />
          <span className="tools-result-count">{data?.count ?? 0} 个结果</span>
        </div>

        {loading ? <LoadingBlock /> : (
          <ToolTable
            section={section}
            data={data}
            summaryOverrides={summaryOverrides}
            onRevealSummary={ensureChineseSummary}
            onAction={setAction}
            selectedIds={selectedIds}
            onSelectTool={toggleSelected}
            onSelectAll={toggleAllVisible}
          />
        )}
        <PaginationBar
          count={data?.count ?? 0}
          page={page}
          pageSize={pageSize}
          selectedCount={selectedTools.length}
          onPage={setPage}
          onPageSize={(next) => {
            setPageSize(next)
            setPage(1)
          }}
        />
      </section>

      {manualOpen && (
        <ToolModal title={section === 'team' ? '手动导入团队工具' : '手动添加 GitHub 工具'} onClose={() => setManualOpen(false)}>
          <form className="tools-modal-form" onSubmit={doPreview}>
            {manualError && <p className="tools-modal-error">{manualError}</p>}
            <label>
              <span>GitHub 仓库地址</span>
              <input value={manualRepoURL} onChange={(e) => setManualRepoURL(e.target.value)} placeholder="https://github.com/owner/repo" />
            </label>
            <p className="tools-field-help">仅支持公开 GitHub 仓库</p>
            <label>
              <span>操作人</span>
              <input value={manualActor} onChange={(e) => setManualActor(e.target.value)} placeholder="手动填写" />
            </label>
            {section === 'team' && (
              <>
                <label>
                  <span>Cooper 文档</span>
                  <input value={manualCooperUrl} onChange={(e) => setManualCooperUrl(e.target.value)} placeholder="可选，https://cooper.didichuxing.com/didocs/..." />
                </label>
                <label>
                  <span>团队使用说明</span>
                  <textarea value={manualSummary} onChange={(e) => setManualSummary(e.target.value)} placeholder="可选，说明团队为什么使用它" />
                </label>
              </>
            )}
            {preview && <ManualPreview preview={preview} />}
            {preview && (
              <div className="tools-classification-mode">
                <span>使用场景分类</span>
                <label>
                  <input type="radio" name="manual-purpose-mode" checked={manualPurposeMode === 'ai'} onChange={() => setManualPurposeMode('ai')} />
                  AI 自动分类
                </label>
                <label>
                  <input type="radio" name="manual-purpose-mode" checked={manualPurposeMode === 'manual'} onChange={() => setManualPurposeMode('manual')} />
                  手动分类
                </label>
                {manualPurposeMode === 'ai' ? (
                  <div className="tools-classification-preview">
                    <span>AI 预估</span>
                    <PurposeChips tags={preview.purposeTags ?? []} manual={false} />
                  </div>
                ) : (
                  <TagEditor
                    label="手动选择使用场景"
                    selected={manualPurposeTags}
                    options={allPurposeTags}
                    onChange={setManualPurposeTags}
                  />
                )}
              </div>
            )}
            <div className="tools-modal-actions">
              <button type="button" className="tools-btn" onClick={() => setManualOpen(false)}>取消</button>
              <button type="submit" className="tools-btn" disabled={manualLoading}>{manualLoading ? '同步中...' : '预览'}</button>
              <button type="button" className="tools-btn primary" onClick={doManualAdd} disabled={manualLoading || !preview}>
                {section === 'team' ? '加入团队工具' : '加入已发现'}
              </button>
            </div>
          </form>
        </ToolModal>
      )}

      {action && (
        <ToolActionPanel
          key={`${action.kind}-${action.tool.id}`}
          action={action}
          onCancel={() => setAction(null)}
          onDone={async (message) => {
            setAction(null)
            onNotice(message)
            await load()
          }}
          onError={onError}
          purposeOptions={allPurposeTags}
        />
      )}

      {batchAction && (
        <BatchActionPanel
          key={`${batchAction.kind}-${batchAction.tools.map((tool) => tool.id).join('-')}`}
          action={batchAction}
          onCancel={() => setBatchAction(null)}
          onRefresh={async (message) => {
            onNotice(message)
            await load()
          }}
          onDone={async (message) => {
            setBatchAction(null)
            setSelectedToolMap({})
            onNotice(message)
            await load()
          }}
          onError={onError}
        />
      )}
    </ToolShell>
  )
}

function PaginationBar({ count, page, pageSize, selectedCount, onPage, onPageSize }: {
  count: number
  page: number
  pageSize: number
  selectedCount: number
  onPage: (page: number) => void
  onPageSize: (pageSize: number) => void
}) {
  if (count <= 0) return null
  const totalPages = Math.max(1, Math.ceil(count / pageSize))
  const safePage = Math.min(page, totalPages)
  const start = (safePage - 1) * pageSize + 1
  const end = Math.min(count, safePage * pageSize)
  return (
    <div className="tools-pagination">
      <span>
        第 {start}-{end} 条 / 共 {count} 条
        {selectedCount > 0 && <strong>，已跨页选择 {selectedCount} 条</strong>}
      </span>
      <div className="tools-pagination-controls">
        <label>
          <span>每页</span>
          <select aria-label="每页数量" value={pageSize} onChange={(e) => onPageSize(Number(e.target.value))}>
            {PAGE_SIZE_OPTIONS.map((size) => <option key={size} value={size}>{size}</option>)}
          </select>
        </label>
        <button type="button" disabled={safePage <= 1} onClick={() => onPage(safePage - 1)}>上一页</button>
        <span>第 {safePage} / {totalPages} 页</span>
        <button type="button" disabled={safePage >= totalPages} onClick={() => onPage(safePage + 1)}>下一页</button>
      </div>
    </div>
  )
}

function BatchActionBar({ section, selectedTools, onBatchAction, onClear }: {
  section: Exclude<ToolsSection, 'configs'>
  selectedTools: ToolItem[]
  onBatchAction: (action: BatchToolAction) => void
  onClear: () => void
}) {
  if (!selectedTools.length) return null
  return (
    <div className="tools-batch-actions" aria-label="批量操作">
      <span>{selectedTools.length} 已选</span>
      {section === 'discovered' && (
        <>
          <button type="button" onClick={() => onBatchAction({ kind: 'batch-start', tools: selectedTools })}>批量测试</button>
          <button type="button" className="danger" onClick={() => onBatchAction({ kind: 'batch-exclude', tools: selectedTools })}>批量删除</button>
        </>
      )}
      {section === 'evaluating' && (
        <>
          <button type="button" onClick={() => onBatchAction({ kind: 'batch-include', tools: selectedTools })}>批量纳入</button>
          <button type="button" className="danger" onClick={() => onBatchAction({ kind: 'batch-reject', tools: selectedTools })}>批量不纳入</button>
        </>
      )}
      {section === 'team' && (
        <>
          <button type="button" onClick={() => onBatchAction({ kind: 'batch-team-edit', tools: selectedTools })}>批量修改</button>
          <button type="button" className="danger" onClick={() => onBatchAction({ kind: 'batch-team-delete', tools: selectedTools })}>批量删除</button>
        </>
      )}
      <button type="button" className="muted" onClick={onClear}>取消选择</button>
    </div>
  )
}

function ToolTable({ section, data, summaryOverrides, onRevealSummary, onAction, selectedIds, onSelectTool, onSelectAll }: {
  section: Exclude<ToolsSection, 'configs'>
  data: ToolList | null
  summaryOverrides: Record<string, string>
  onRevealSummary: (tool: ToolItem) => void
  onAction: (action: ToolAction) => void
  selectedIds: string[]
  onSelectTool: (toolID: string, checked: boolean) => void
  onSelectAll: (checked: boolean) => void
}) {
  const items = data?.items ?? []
  const selected = new Set(selectedIds)
  const allSelected = items.length > 0 && items.every((tool) => selected.has(tool.id))
  if (!items.length) return <EmptyBlock text="暂无工具" />
  if (section === 'discovered') {
    return (
      <div className="tools-table-wrap">
        <table className="tools-data-table">
          <thead>
            <tr>
              <th className="tools-select-col">
                <input type="checkbox" aria-label="选择全部工具百宝箱条目" checked={allSelected} onChange={(e) => onSelectAll(e.target.checked)} />
              </th>
              <th>工具</th>
              <th>介绍</th>
              <th>用途</th>
              <th>发现来源</th>
              <th>Stars</th>
              <th>7 日增长</th>
              <th>最近提交</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {items.map((tool) => (
              <tr key={tool.id}>
                <td className="tools-select-col">
                  <input type="checkbox" aria-label={`选择 ${tool.githubFullName}`} checked={selected.has(tool.id)} onChange={(e) => onSelectTool(tool.id, e.target.checked)} />
                </td>
                <td><ToolIdentity tool={tool} /></td>
                <td><ToolDescription text={toolSummaryText(tool, summaryOverrides)} onReveal={() => onRevealSummary(tool)} /></td>
                <td><PurposeChips tags={tool.purposeTags} manual={tool.purposeTagsManuallySet} /></td>
                <td><SourceChips tool={tool} /></td>
                <td className="tools-strong">{formatCompactNumber(tool.stars)}</td>
                <td className={tool.stars7d ? 'tools-growth' : 'tools-muted'}>{formatGrowth(tool.stars7d)}</td>
                <td><DateStack main={formatPushedAt(tool.pushedAt)} sub={`发现 ${formatShortDateTime(tool.firstDiscoveredAt)}`} /></td>
                <td>
                  <div className="tools-row-actions">
                    <RowIconButton label="开始测评" icon="☑" onClick={() => onAction({ kind: 'start', tool })} />
                    <RowIconButton label="分类" icon="⌁" onClick={() => onAction({ kind: 'purpose', tool })} />
                    <RowIconButton label="不处理" icon="⌫" danger onClick={() => onAction({ kind: 'exclude', tool })} />
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    )
  }
  if (section === 'evaluating') {
    return (
      <div className="tools-table-wrap">
        <table className="tools-data-table evaluating">
          <thead>
            <tr>
              <th className="tools-select-col">
                <input type="checkbox" aria-label="选择全部测评中工具" checked={allSelected} onChange={(e) => onSelectAll(e.target.checked)} />
              </th>
              <th>工具</th>
              <th>介绍</th>
              <th>用途</th>
              <th>测评人</th>
              <th>Cooper 测评记录</th>
              <th>开始时间</th>
              <th>测评结论</th>
            </tr>
          </thead>
          <tbody>
            {items.map((tool) => (
              <tr key={tool.id}>
                <td className="tools-select-col">
                  <input type="checkbox" aria-label={`选择 ${tool.githubFullName}`} checked={selected.has(tool.id)} onChange={(e) => onSelectTool(tool.id, e.target.checked)} />
                </td>
                <td><ToolIdentity tool={tool} /></td>
                <td><ToolDescription text={toolSummaryText(tool, summaryOverrides)} onReveal={() => onRevealSummary(tool)} /></td>
                <td><PurposeChips tags={tool.purposeTags} manual={tool.purposeTagsManuallySet} /></td>
                <td><EvaluatorBadge name={tool.evaluation?.evaluator || '待填写'} /></td>
                <td>
                  {tool.evaluation?.cooperUrl ? (
                    <a className="tools-doc-link" href={tool.evaluation.cooperUrl} target="_blank" rel="noreferrer">查看测评文档 ↗</a>
                  ) : (
                    <button type="button" className="tools-link-action" onClick={() => onAction({ kind: 'cooper', tool })}>关联文档</button>
                  )}
                </td>
                <td>{formatMonthDay(tool.evaluation?.startedAt)}</td>
                <td>
                  <div className="tools-row-actions">
                    <RowIconButton label="✓ 纳入" icon="✓" onClick={() => onAction({ kind: 'include', tool })} />
                    <RowIconButton label="分类" icon="⌁" onClick={() => onAction({ kind: 'purpose', tool })} />
                    <RowIconButton label="不纳入" icon="×" danger onClick={() => onAction({ kind: 'reject', tool })} />
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    )
  }
  return (
    <div className="tools-table-wrap">
      <table className="tools-data-table team">
        <thead>
          <tr>
            <th className="tools-select-col">
              <input type="checkbox" aria-label="选择全部团队工具" checked={allSelected} onChange={(e) => onSelectAll(e.target.checked)} />
            </th>
            <th>工具</th>
            <th>团队使用说明</th>
            <th>用途</th>
            <th>测评人</th>
            <th>纳入时间</th>
            <th>相关链接</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          {items.map((tool) => (
            <tr key={tool.id}>
              <td className="tools-select-col">
                <input type="checkbox" aria-label={`选择 ${tool.githubFullName}`} checked={selected.has(tool.id)} onChange={(e) => onSelectTool(tool.id, e.target.checked)} />
              </td>
              <td><ToolIdentity tool={tool} /></td>
              <td><ToolDescription text={tool.finalSummary || toolSummaryText(tool, summaryOverrides, '暂无说明')} full onReveal={() => onRevealSummary(tool)} /></td>
              <td><PurposeChips tags={tool.purposeTags} manual={tool.purposeTagsManuallySet} /></td>
              <td>{tool.evaluation?.evaluator || '暂无'}</td>
              <td>{formatMonthDay(tool.includedAt)}</td>
              <td>
                <div className="tools-link-stack">
                  <a href={tool.githubUrl} target="_blank" rel="noreferrer">GitHub</a>
                  {tool.evaluation?.cooperUrl && <a href={tool.evaluation.cooperUrl} target="_blank" rel="noreferrer">Cooper 测评</a>}
                </div>
              </td>
              <td>
                <div className="tools-row-actions">
                  <RowIconButton label="编辑" icon="✎" onClick={() => onAction({ kind: 'team-edit', tool })} />
                  <RowIconButton label="分类" icon="⌁" onClick={() => onAction({ kind: 'purpose', tool })} />
                  <RowIconButton label="删除" icon="⌫" danger onClick={() => onAction({ kind: 'team-delete', tool })} />
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function RowIconButton({ label, icon, danger, onClick }: {
  label: string
  icon: string
  danger?: boolean
  onClick: () => void
}) {
  return (
    <button type="button" className={danger ? 'danger' : undefined} aria-label={label} title={label} onClick={onClick}>
      <span aria-hidden>{icon}</span>
    </button>
  )
}

function ToolActionPanel({ action, onCancel, onDone, onError, purposeOptions }: {
  action: ToolAction
  onCancel: () => void
  onDone: (message: string) => Promise<void>
  onError: (v: string) => void
  purposeOptions: string[]
}) {
  const [evaluator, setEvaluator] = useState('')
  const [operator, setOperator] = useState('')
  const [cooperUrl, setCooperUrl] = useState(action.tool.evaluation?.cooperUrl ?? '')
  const [summary, setSummary] = useState(action.tool.finalSummary ?? action.tool.currentSummary ?? '')
  const [purposeTags, setPurposeTags] = useState<string[]>(action.tool.purposeTags ?? [])
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)
  const [modalError, setModalError] = useState('')

  useEffect(() => {
    if (!modalError) return
    const timer = window.setTimeout(() => setModalError(''), 3000)
    return () => window.clearTimeout(timer)
  }, [modalError])

  function requireModalField(value: string, message: string): boolean {
    if (value.trim()) return true
    setModalError(message)
    return false
  }

  async function submit(e: FormEvent) {
    e.preventDefault()
    if ((action.kind === 'start') && !requireModalField(evaluator, '请填写测评人')) return
    if (!requireModalField(operator, '请填写操作人')) return
    if (action.kind === 'cooper' && !requireModalField(cooperUrl, '请填写 Cooper 链接')) return
    if ((action.kind === 'exclude' || action.kind === 'reject' || action.kind === 'team-delete') && !requireModalField(reason, '请填写原因')) return
    setBusy(true)
    onError('')
    try {
      if (action.kind === 'start') {
        await startToolEvaluation({ toolId: action.tool.id, evaluator, operator, cooperUrl })
        await onDone('已开始测评')
      } else if (action.kind === 'exclude') {
        if (!window.confirm(`确认不处理「${action.tool.name}」？`)) return
        await excludeDiscoveredTool(action.tool.id, reason, operator)
        await onDone('已标记不处理')
      } else if (action.kind === 'cooper') {
        await updateToolEvaluationCooperURL(action.tool.id, cooperUrl, operator)
        await onDone('Cooper 链接已更新')
      } else if (action.kind === 'include') {
        await finishToolEvaluation({
          toolId: action.tool.id,
          evaluationId: action.tool.evaluation?.id ?? '',
          result: 'included',
          operator,
          finalSummary: summary,
        })
        await onDone('已纳入团队工具')
      } else if (action.kind === 'reject') {
        await finishToolEvaluation({
          toolId: action.tool.id,
          evaluationId: action.tool.evaluation?.id ?? '',
          result: 'excluded',
          operator,
          notIncludedReason: reason,
        })
        await onDone('已完成为不纳入')
      } else if (action.kind === 'purpose') {
        await updateToolPurposeTags({
          toolId: action.tool.id,
          actor: operator,
          purposeTags,
        })
        await onDone('用途分类已更新')
      } else if (action.kind === 'team-edit') {
        await updateTeamTool({
          toolId: action.tool.id,
          operator,
          cooperUrl,
          finalSummary: summary,
        })
        await onDone('团队工具已更新')
      } else {
        await deleteTeamTool(action.tool.id, operator, reason)
        await onDone('团队工具已删除')
      }
    } catch (err) {
      onError(formatToolError(err))
    } finally {
      setBusy(false)
    }
  }

  const title = {
    start: '开始测评',
    exclude: '不处理',
    cooper: '关联 Cooper 文档',
    include: '纳入团队工具',
    reject: '不纳入',
    purpose: '修正用途分类',
    'team-edit': '编辑团队工具',
    'team-delete': '删除团队工具',
  }[action.kind]

  return (
    <ToolModal title={`${title} · ${action.tool.githubFullName}`} onClose={onCancel}>
      <form className="tools-modal-form" onSubmit={submit}>
        {modalError && <p className="tools-modal-error">{modalError}</p>}
        {action.kind === 'start' && (
          <div className="tools-form-grid">
            <label>
              <span>测评人</span>
              <input value={evaluator} onChange={(e) => setEvaluator(e.target.value)} placeholder="手动填写" />
            </label>
            <label>
              <span>操作人</span>
              <input value={operator} onChange={(e) => setOperator(e.target.value)} placeholder="手动填写" />
            </label>
          </div>
        )}
        {action.kind !== 'start' && (
          <label>
            <span>操作人</span>
            <input value={operator} onChange={(e) => setOperator(e.target.value)} placeholder="手动填写" />
          </label>
        )}
        {(action.kind === 'start' || action.kind === 'cooper' || action.kind === 'team-edit') && (
          <label>
            <span>{action.kind === 'team-edit' ? 'Cooper 文档' : 'Cooper 链接'}</span>
            <input value={cooperUrl} onChange={(e) => setCooperUrl(e.target.value)} placeholder="https://cooper.didichuxing.com/didocs/..." />
          </label>
        )}
        {(action.kind === 'include' || action.kind === 'team-edit') && (
          <label>
            <span>团队使用说明</span>
            <textarea value={summary} onChange={(e) => setSummary(e.target.value)} placeholder="它是什么、解决什么问题、适合什么场景" />
          </label>
        )}
        {action.kind === 'purpose' && (
          <TagEditor
            label="用途分类"
            selected={purposeTags}
            options={mergeTagOptions(purposeOptions, action.tool.purposeTags)}
            onChange={setPurposeTags}
          />
        )}
        {(action.kind === 'exclude' || action.kind === 'reject' || action.kind === 'team-delete') && (
          <label>
            <span>{action.kind === 'team-delete' ? '删除原因' : action.kind === 'exclude' ? '不处理原因' : '不纳入原因'}</span>
            <textarea value={reason} onChange={(e) => setReason(e.target.value)} placeholder="记录原因，便于后续追溯" />
          </label>
        )}
        <div className="tools-modal-actions">
          <button type="button" className="tools-btn" onClick={onCancel}>取消</button>
          <button className="tools-btn primary" type="submit" disabled={busy}>{busy ? '处理中...' : '提交'}</button>
        </div>
      </form>
    </ToolModal>
  )
}

function BatchActionPanel({ action, onCancel, onRefresh, onDone, onError }: {
  action: BatchToolAction
  onCancel: () => void
  onRefresh: (message: string) => Promise<void>
  onDone: (message: string) => Promise<void>
  onError: (v: string) => void
}) {
  const [evaluator, setEvaluator] = useState('')
  const [operator, setOperator] = useState('')
  const [sharedReason, setSharedReason] = useState('')
  const [rows, setRows] = useState<BatchRowState[]>(() => action.tools.map((tool) => ({
    toolId: tool.id,
    cooperUrl: tool.evaluation?.cooperUrl ?? '',
    finalSummary: tool.finalSummary ?? tool.currentSummary ?? '',
    reason: '',
  })))
  const [busy, setBusy] = useState(false)
  const [modalError, setModalError] = useState('')

  const toolsByID = useMemo(() => new Map(action.tools.map((tool) => [tool.id, tool])), [action.tools])

  useEffect(() => {
    if (!modalError) return
    const timer = window.setTimeout(() => setModalError(''), 3000)
    return () => window.clearTimeout(timer)
  }, [modalError])

  function updateRow(toolID: string, field: keyof Omit<BatchRowState, 'toolId'>, value: string) {
    setRows((prev) => prev.map((row) => row.toolId === toolID ? { ...row, [field]: value } : row))
  }

  function rowFor(toolID: string): BatchRowState {
    return rows.find((row) => row.toolId === toolID) ?? { toolId: toolID, cooperUrl: '', finalSummary: '', reason: '' }
  }

  function requireBatchField(value: string, message: string): boolean {
    if (value.trim()) return true
    setModalError(message)
    return false
  }

  function validateRows(): boolean {
    if (action.kind === 'batch-start' && !requireBatchField(evaluator, '请填写测评人')) return false
    if (!requireBatchField(operator, '请填写操作人')) return false
    if ((action.kind === 'batch-exclude' || action.kind === 'batch-team-delete') && !requireBatchField(sharedReason, '请填写删除原因')) return false

    if (action.kind === 'batch-include') {
      for (const row of rows) {
        const tool = toolsByID.get(row.toolId)
        const name = tool?.githubFullName || '所选工具'
        if (!row.cooperUrl.trim() && !tool?.evaluation?.cooperUrl) {
          setModalError(`请填写 ${name} 的 Cooper 链接`)
          return false
        }
        const summary = row.finalSummary.trim()
        if (summary.length < 20) {
          setModalError(`${name} 的团队使用说明至少 20 字`)
          return false
        }
        if (summary.length > 1000) {
          setModalError(`${name} 的团队使用说明最多 1000 字`)
          return false
        }
      }
    }

    if (action.kind === 'batch-reject') {
      for (const row of rows) {
        if (!row.reason.trim()) {
          setModalError(`请填写 ${toolsByID.get(row.toolId)?.githubFullName || '所选工具'} 的不纳入原因`)
          return false
        }
      }
    }

    if (action.kind === 'batch-team-edit') {
      for (const row of rows) {
        if (row.finalSummary.trim().length > 1000) {
          setModalError(`${toolsByID.get(row.toolId)?.githubFullName || '所选工具'} 的团队使用说明最多 1000 字`)
          return false
        }
      }
    }
    return true
  }

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!validateRows()) return
    setBusy(true)
    setModalError('')
    onError('')
    const failures: string[] = []
    let success = 0
    for (const tool of action.tools) {
      const row = rowFor(tool.id)
      try {
        if (action.kind === 'batch-start') {
          await startToolEvaluation({
            toolId: tool.id,
            evaluator,
            operator,
            cooperUrl: row.cooperUrl.trim() || undefined,
          })
        } else if (action.kind === 'batch-exclude') {
          await excludeDiscoveredTool(tool.id, sharedReason, operator)
        } else if (action.kind === 'batch-include') {
          const nextCooper = row.cooperUrl.trim()
          if (nextCooper && nextCooper !== (tool.evaluation?.cooperUrl ?? '')) {
            await updateToolEvaluationCooperURL(tool.id, nextCooper, operator)
          }
          await finishToolEvaluation({
            toolId: tool.id,
            evaluationId: tool.evaluation?.id ?? '',
            result: 'included',
            operator,
            finalSummary: row.finalSummary.trim(),
          })
        } else if (action.kind === 'batch-reject') {
          await finishToolEvaluation({
            toolId: tool.id,
            evaluationId: tool.evaluation?.id ?? '',
            result: 'excluded',
            operator,
            notIncludedReason: row.reason.trim(),
          })
        } else if (action.kind === 'batch-team-edit') {
          await updateTeamTool({
            toolId: tool.id,
            operator,
            cooperUrl: row.cooperUrl.trim() || undefined,
            finalSummary: row.finalSummary.trim() || undefined,
          })
        } else {
          await deleteTeamTool(tool.id, operator, sharedReason)
        }
        success += 1
      } catch (err) {
        failures.push(`${tool.githubFullName}：${formatToolError(err)}`)
      }
    }
    setBusy(false)

    const label = batchDoneLabel(action.kind)
    if (failures.length) {
      const message = `${label}部分完成：成功 ${success}，失败 ${failures.length}`
      setModalError(failures.slice(0, 3).join('\n'))
      onError(message)
      if (success > 0) await onRefresh(message)
      return
    }
    await onDone(`${label}完成：${success} 个工具`)
  }

  const title = batchTitle(action.kind)
  return (
    <ToolModal title={`${title} · ${action.tools.length} 个工具`} onClose={onCancel}>
      <form className="tools-modal-form tools-batch-modal" onSubmit={submit}>
        {modalError && <p className="tools-modal-error">{modalError}</p>}
        {(action.kind === 'batch-start') && (
          <div className="tools-form-grid">
            <label>
              <span>测评人</span>
              <input value={evaluator} onChange={(e) => setEvaluator(e.target.value)} placeholder="手动填写" />
            </label>
            <label>
              <span>操作人</span>
              <input value={operator} onChange={(e) => setOperator(e.target.value)} placeholder="手动填写" />
            </label>
          </div>
        )}
        {action.kind !== 'batch-start' && (
          <label>
            <span>操作人</span>
            <input value={operator} onChange={(e) => setOperator(e.target.value)} placeholder="手动填写" />
          </label>
        )}
        {(action.kind === 'batch-exclude' || action.kind === 'batch-team-delete') && (
          <label>
            <span>{action.kind === 'batch-team-delete' ? '删除原因' : '不处理原因'}</span>
            <textarea value={sharedReason} onChange={(e) => setSharedReason(e.target.value)} placeholder="批量删除原因会写入每个工具的操作记录" />
          </label>
        )}
        {(action.kind === 'batch-exclude' || action.kind === 'batch-team-delete') ? (
          <BatchToolList tools={action.tools} />
        ) : (
          <div className="tools-batch-rows">
            {action.tools.map((tool) => {
              const row = rowFor(tool.id)
              return (
                <div className="tools-batch-row" key={tool.id}>
                  <ToolIdentity tool={tool} />
                  {(action.kind === 'batch-start' || action.kind === 'batch-include' || action.kind === 'batch-team-edit') && (
                    <label>
                      <span>{action.kind === 'batch-start' ? 'Cooper 链接' : 'Cooper 文档'}</span>
                      <input value={row.cooperUrl} onChange={(e) => updateRow(tool.id, 'cooperUrl', e.target.value)} placeholder={action.kind === 'batch-start' ? '可选，https://cooper.didichuxing.com/didocs/...' : 'https://cooper.didichuxing.com/didocs/...'} />
                    </label>
                  )}
                  {(action.kind === 'batch-include' || action.kind === 'batch-team-edit') && (
                    <label>
                      <span>团队使用说明</span>
                      <textarea value={row.finalSummary} onChange={(e) => updateRow(tool.id, 'finalSummary', e.target.value)} placeholder="它是什么、解决什么问题、适合什么场景" />
                    </label>
                  )}
                  {action.kind === 'batch-reject' && (
                    <label>
                      <span>不纳入原因</span>
                      <textarea value={row.reason} onChange={(e) => updateRow(tool.id, 'reason', e.target.value)} placeholder="分别记录不纳入原因" />
                    </label>
                  )}
                </div>
              )
            })}
          </div>
        )}
        <div className="tools-modal-actions">
          <button type="button" className="tools-btn" onClick={onCancel}>取消</button>
          <button className="tools-btn primary" type="submit" disabled={busy}>{busy ? '处理中...' : '提交'}</button>
        </div>
      </form>
    </ToolModal>
  )
}

function BatchToolList({ tools }: { tools: ToolItem[] }) {
  return (
    <div className="tools-batch-list">
      {tools.map((tool) => (
        <div className="tools-batch-list-item" key={tool.id}>
          <ToolLogo name={tool.name} />
          <div>
            <strong>{tool.name}</strong>
            <span>{tool.githubFullName}</span>
          </div>
        </div>
      ))}
    </div>
  )
}

function batchTitle(kind: BatchToolAction['kind']): string {
  return {
    'batch-start': '批量开始测评',
    'batch-exclude': '批量删除工具百宝箱条目',
    'batch-include': '批量纳入团队工具',
    'batch-reject': '批量不纳入',
    'batch-team-edit': '批量修改团队工具',
    'batch-team-delete': '批量删除团队工具',
  }[kind]
}

function batchDoneLabel(kind: BatchToolAction['kind']): string {
  return {
    'batch-start': '批量测评',
    'batch-exclude': '批量删除',
    'batch-include': '批量纳入',
    'batch-reject': '批量不纳入',
    'batch-team-edit': '批量修改',
    'batch-team-delete': '批量删除',
  }[kind]
}

function ToolModal({ title, onClose, children }: {
  title: string
  onClose: () => void
  children: ReactNode
}) {
  return (
    <div className="tools-modal-backdrop">
      <section className="tools-modal" role="dialog" aria-modal="true" aria-label={title}>
        <header className="tools-modal-head">
          <h2>{title}</h2>
          <button type="button" className="tools-modal-close" aria-label="关闭" onClick={onClose}>×</button>
        </header>
        <div className="tools-modal-body">{children}</div>
      </section>
    </div>
  )
}

function SummaryStrip({ aside, children }: { aside?: string; children: ReactNode }) {
  return (
    <div className="tools-summary-strip">
      <div className="tools-summary-items">{children}</div>
      {aside && <span>{aside}</span>}
    </div>
  )
}

function SummaryMetric({ value, label }: { value: string | number; label: string }) {
  return (
    <span className="tools-summary-metric">
      <strong>{value}</strong>{label}
    </span>
  )
}

function ConfigFilterBar({
  methodFilter,
  triggerFilter,
  onMethodFilter,
  onTriggerFilter,
}: {
  methodFilter: ConfigMethodFilter
  triggerFilter: ConfigTriggerFilter
  onMethodFilter: (next: ConfigMethodFilter) => void
  onTriggerFilter: (next: ConfigTriggerFilter) => void
}) {
  return (
    <div className="tools-filter-bar">
      <SelectFilter
        label="发现方式"
        options={CONFIG_METHOD_FILTER_OPTIONS}
        value={methodFilter}
        onChange={onMethodFilter}
      />
      <SelectFilter
        label="触发方式"
        options={CONFIG_TRIGGER_FILTER_OPTIONS}
        value={triggerFilter}
        onChange={onTriggerFilter}
      />
    </div>
  )
}

function SelectFilter<T extends string>({
  label,
  options,
  value,
  onChange,
}: {
  label: string
  options: { key: T; label: string }[]
  value: T
  onChange: (next: T) => void
}) {
  const id = `tools-filter-${label}`
  return (
    <label className="tools-filter-select" htmlFor={id}>
      <span>{label}</span>
      <select id={id} aria-label={label} value={value} onChange={(event) => onChange(event.target.value as T)}>
        {options.map((opt) => (
          <option key={opt.key} value={opt.key}>{opt.label}</option>
        ))}
      </select>
    </label>
  )
}

function ToolIdentity({ tool }: { tool: ToolItem }) {
  const content = (
    <>
      <ToolLogo name={tool.name} />
      <div>
        <div className="tools-tool-name">
          <strong>{tool.name}</strong>
        </div>
        <span>{tool.githubFullName}</span>
      </div>
    </>
  )
  if (isGitHubURL(tool.githubUrl)) {
    return (
      <a className="tools-identity tools-identity-link" href={tool.githubUrl} target="_blank" rel="noreferrer" aria-label={`打开 GitHub：${tool.githubFullName}`}>
        {content}
      </a>
    )
  }
  return (
    <div className="tools-identity">
      {content}
    </div>
  )
}

function ToolDescription({ text, full = false, onReveal }: {
  text: string
  full?: boolean
  onReveal: () => void
}) {
  const ref = useRef<HTMLParagraphElement | null>(null)
  const [tooltip, setTooltip] = useState<{ left: number; top: number; width: number; above: boolean } | null>(null)

  function reveal() {
    onReveal()
    const rect = ref.current?.getBoundingClientRect()
    if (!rect || typeof window === 'undefined') return
    const width = Math.min(480, Math.max(260, rect.width || 260))
    const left = Math.min(Math.max(12, rect.left), Math.max(12, window.innerWidth - width - 12))
    const above = rect.bottom + 180 > window.innerHeight && rect.top > 180
    setTooltip({
      left,
      top: above ? rect.top - 8 : rect.bottom + 8,
      width,
      above,
    })
  }

  return (
    <>
      <p
        ref={ref}
        className={`tools-table-desc ${full ? 'full' : ''}`}
        title={text}
        tabIndex={0}
        onBlur={() => setTooltip(null)}
        onFocus={reveal}
        onMouseEnter={reveal}
        onMouseLeave={() => setTooltip(null)}
      >
        {text}
      </p>
      {tooltip && typeof document !== 'undefined' && createPortal(
        <div
          className="tools-floating-tooltip"
          style={{
            left: tooltip.left,
            top: tooltip.top,
            width: tooltip.width,
            transform: tooltip.above ? 'translateY(-100%)' : undefined,
          }}
        >
          {text}
        </div>,
        document.body,
      )}
    </>
  )
}

function ToolLogo({ name }: { name: string }) {
  const style = { '--tool-color': toolColor(name) } as CSSProperties
  return <div className="tools-tool-logo" style={style}>{initials(name)}</div>
}

function SourceChips({ tool }: { tool: ToolItem }) {
  return (
    <div className="tools-source-chips">
      {tool.sources.map((source) => (
        <span key={`${source.sourceType}-${source.term}`} className={source.sourceType}>
          {sourceLabel(source.sourceType)} · {source.term}
        </span>
      ))}
    </div>
  )
}

function PurposeChips({ tags, manual }: { tags: string[]; manual: boolean }) {
  const normalized = normalizeTags(tags)
  if (!normalized.length) return <span className="tools-muted">未分类</span>
  return (
    <div className="tools-purpose-chips" title={manual ? '人工修正' : '自动分类'}>
      {normalized.map((tag) => <span key={tag}>{tag}</span>)}
    </div>
  )
}

function TagEditor({
  label,
  selected,
  options,
  onChange,
}: {
  label: string
  selected: string[]
  options: string[]
  onChange: (tags: string[]) => void
}) {
  const [selectedOption, setSelectedOption] = useState('')
  const [customTag, setCustomTag] = useState('')
  const normalizedSelected = normalizeTags(selected)
  const availableOptions = mergeTagOptions(options).filter((tag) => !normalizedSelected.includes(tag))

  function addTag(tag: string) {
    const next = normalizeTag(tag)
    if (!next || normalizedSelected.includes(next)) return
    onChange([...normalizedSelected, next].slice(0, 8))
    setSelectedOption('')
    setCustomTag('')
  }

  function removeTag(tag: string) {
    onChange(normalizedSelected.filter((item) => item !== tag))
  }

  return (
    <div className="tools-tag-editor">
      <span>{label}</span>
      <div className="tools-tag-selected">
        {normalizedSelected.length ? normalizedSelected.map((tag) => (
          <button type="button" key={tag} onClick={() => removeTag(tag)} title="点击移除">
            {tag}<i aria-hidden>×</i>
          </button>
        )) : <em>未分类</em>}
      </div>
      <div className="tools-tag-controls">
        <select aria-label="选择用途分类" value={selectedOption} onChange={(e) => setSelectedOption(e.target.value)}>
          <option value="">选择已有分类</option>
          {availableOptions.map((tag) => <option key={tag} value={tag}>{tag}</option>)}
        </select>
        <button type="button" className="tools-btn" onClick={() => addTag(selectedOption)} disabled={!selectedOption}>添加</button>
      </div>
      <div className="tools-tag-controls">
        <input value={customTag} onChange={(e) => setCustomTag(e.target.value)} placeholder="新增分类，如：会议纪要" />
        <button type="button" className="tools-btn" onClick={() => addTag(customTag)} disabled={!customTag.trim()}>新增</button>
      </div>
      <p className="tools-field-help">按使用场景分类；没有合适场景可以保持未分类。</p>
    </div>
  )
}

function ManualPreview({ preview }: { preview: GitHubRepositoryPreview }) {
  return (
    <div className="manual-preview-card">
      <ToolLogo name={preview.name} />
      <div>
        <strong>{preview.name}</strong>
        <span>{preview.fullName} · 仓库信息将在提交后同步</span>
      </div>
    </div>
  )
}

function EvaluatorBadge({ name }: { name: string }) {
  return (
    <span className="tools-evaluator">
      <i>{initials(name)}</i>{name}
    </span>
  )
}

function DateStack({ main, sub }: { main: string; sub: string }) {
  return (
    <div className="tools-date-stack">
      <strong>{main}</strong>
      <span>{sub}</span>
    </div>
  )
}

function LoadingBlock() {
  return <div className="tools-empty">加载中...</div>
}

function EmptyBlock({ text }: { text: string }) {
  return <div className="tools-empty">{text}</div>
}

function emptyConfigForm(actor: string): ConfigFormState {
  return {
    id: '',
    name: '',
    method: 'keyword',
    termsText: '',
    triggerMode: 'manual',
    enabled: true,
    actor,
  }
}

function emptySettingsForm(actor: string, settings: ToolRuntimeSettings | null): SettingsFormState {
  const safe = normalizeSettings(settings)
  return {
    tokenId: '',
    tokenName: '',
    tokenDescription: '',
    tokenValue: '',
    tokenTestUrl: '',
    githubBaseUrl: safe.githubBaseUrl,
    useEnterpriseGitHub: safe.githubBaseUrl !== '' && safe.githubBaseUrl !== 'https://api.github.com',
    githubTokenStrategy: safe.githubTokenStrategy,
    githubActiveTokenIndex: String(safe.githubActiveTokenIndex),
    includeDefaultGitHubTokens: safe.includeDefaultGitHubTokens,
    actor,
  }
}

function normalizeSettings(settings: ToolRuntimeSettings | null | undefined): ToolRuntimeSettings {
  return {
    githubTokenCount: settings?.githubTokenCount ?? 0,
    githubTokens: settings?.githubTokens ?? [],
    defaultGitHubTokenCount: settings?.defaultGitHubTokenCount ?? 0,
    defaultGitHubTokens: settings?.defaultGitHubTokens ?? [],
    githubBaseUrl: settings?.githubBaseUrl ?? '',
    githubTokenStrategy: normalizeGitHubTokenStrategy(settings?.githubTokenStrategy),
    githubActiveTokenIndex: settings?.githubActiveTokenIndex ?? 0,
    includeDefaultGitHubTokens: settings?.includeDefaultGitHubTokens ?? true,
    githubMaxPages: settings?.githubMaxPages ?? 5,
    githubPerPage: settings?.githubPerPage ?? 100,
    githubRequestIntervalMs: settings?.githubRequestIntervalMs ?? 2000,
    starSnapshotLimit: settings?.starSnapshotLimit ?? 200,
    updatedBy: settings?.updatedBy,
    updatedAt: settings?.updatedAt,
  }
}

function normalizeGitHubTokenStrategy(value: unknown): GitHubTokenStrategy {
  return value === 'fixed' || value === 'failover' || value === 'round_robin' ? value : 'round_robin'
}

function tokenStrategyLabel(value: GitHubTokenStrategy): string {
  const found = GITHUB_TOKEN_STRATEGIES.find((item) => item.value === value)
  return found?.label ?? '轮流使用'
}

function parseActiveTokenIndex(raw: string): number {
  const n = Number(raw)
  return Number.isInteger(n) && n >= 0 ? n : 0
}

function resolveConfigRunState(cfg: DiscoveryConfig, overrides: Record<string, ConfigRunState>): ConfigRunState {
  const override = overrides[cfg.id]
  const serverState = serverConfigRunState(cfg)
  if (override && serverState !== 'paused') return override
  if (serverState !== 'idle') return serverState
  return override ?? 'idle'
}

function serverConfigRunState(cfg: DiscoveryConfig): ConfigRunState {
  if (cfg.lastRunStatus === 'running') return cfg.lastPauseRequested ? 'pausing' : 'running'
  if (cfg.lastRunStatus === 'paused') return 'paused'
  return 'idle'
}

function reconcileConfigRunOverrides(
  overrides: Record<string, ConfigRunState>,
  configs: DiscoveryConfig[],
): Record<string, ConfigRunState> {
  let next = overrides
  const visibleIDs = new Set(configs.map((cfg) => cfg.id))
  function ensureCopy() {
    if (next === overrides) next = { ...overrides }
  }
  for (const id of Object.keys(overrides)) {
    if (!visibleIDs.has(id)) {
      ensureCopy()
      delete next[id]
    }
  }
  for (const cfg of configs) {
    const override = next[cfg.id]
    if (!override) continue
    const serverState = serverConfigRunState(cfg)
    if (serverState === 'idle' || serverState === 'paused' || serverState === override) {
      ensureCopy()
      delete next[cfg.id]
    }
  }
  return next
}

function configRunButtonMeta(state: ConfigRunState): { label: string; title: string; icon: string } {
  if (state === 'running') return { label: '暂停', title: '暂停本次执行', icon: 'Ⅱ' }
  if (state === 'pausing') return { label: '暂停中', title: '正在暂停', icon: '…' }
  if (state === 'paused') return { label: '继续', title: '继续本次执行', icon: '▷' }
  return { label: '执行', title: '执行', icon: '▷' }
}

function configRunProgressText(cfg: DiscoveryConfig, state: ConfigRunState): string {
  if (state === 'idle') return ''
  const pages = cfg.lastPagesScanned ?? 0
  const count = cfg.lastResultCount ?? 0
  const progress = `已扫 ${pages} 页 · 结果 ${count}`
  if (state === 'running') return `运行中 · ${progress}`
  if (state === 'pausing') return `暂停中 · ${progress}`
  return `已暂停 · ${progress}`
}

function statusForSection(section: Exclude<ToolsSection, 'configs'>): ToolStatus {
  if (section === 'evaluating') return 'evaluating'
  if (section === 'team') return 'included'
  return 'discovered'
}

function toolSummaryText(tool: ToolItem, summaryOverrides: Record<string, string>, fallback = '暂无描述'): string {
  return summaryOverrides[tool.id] || tool.currentSummary || tool.description || fallback
}

function mergePurposeOptions(tags: string[], tools: ToolItem[]): { key: PurposeFilter; label: string }[] {
  const fromTools = tools.flatMap((tool) => tool.purposeTags ?? [])
  const options = mergeTagOptions(['all'], DEFAULT_PURPOSE_TAGS, tags, fromTools)
  return options.map((tag) => ({ key: tag, label: tag === 'all' ? '全部用途' : tag }))
}

function mergeTagOptions(...groups: Array<string[] | undefined>): string[] {
  const out: string[] = []
  const seen = new Set<string>()
  for (const group of groups) {
    for (const raw of group ?? []) {
      const tag = normalizeTag(raw)
      if (!tag) continue
      const key = tag.toLowerCase()
      if (seen.has(key)) continue
      seen.add(key)
      out.push(tag)
    }
  }
  return out
}

function normalizeTags(tags: string[] | undefined): string[] {
  return mergeTagOptions(tags).slice(0, 8)
}

function normalizeTag(raw: string): string {
  return raw.trim().replace(/\s+/g, ' ').replace(/^[，,;；、]+|[，,;；、]+$/g, '').slice(0, 24)
}

function hasCJKText(text: string): boolean {
  return /[\u3400-\u9fff]/.test(text)
}

function isGitHubURL(raw: string | undefined): boolean {
  if (!raw) return false
  try {
    const url = new URL(raw)
    return url.hostname === 'github.com' || url.hostname.endsWith('.github.com')
  } catch {
    return false
  }
}

function configRunReasonLabel(reason: string): string {
  const message = extractConfigRunMessage(reason)
  const lower = message.toLowerCase()
  if (
    message === 'GitHub search reached the configured page limit' ||
    message === 'GitHub search returned incomplete results' ||
    lower.includes('rate limit')
  ) {
    return '提示'
  }
  return '失败'
}

function formatConfigRunReason(reason: string): string {
  const message = extractConfigRunMessage(reason)
  const lower = message.toLowerCase()
  if (lower.includes('context canceled') || lower.includes('request cancellation') || lower.includes('running discovery was closed')) {
    return '上次执行中断，已自动结束'
  }
  if (lower.includes('bad credentials') || lower.includes('invalid credentials') || lower.includes('status') && lower.includes('401')) {
    return 'GitHub Token 已失效，请到账号设置测试或更换 Token'
  }
  if (lower.includes('secondary rate limit')) return 'GitHub 暂时限流，请稍后再试'
  if (lower.includes('api rate limit exceeded') || lower.includes('rate limit')) return 'GitHub API 限流，请稍后再试'
  if (message === 'GitHub search reached the configured page limit') return '已达到本次扫描页数上限'
  if (message === 'GitHub search returned incomplete results') return 'GitHub 返回结果不完整'
  return message
}

function extractConfigRunMessage(reason: string): string {
  const trimmed = reason.trim()
  if (!trimmed.startsWith('{')) return reason
  try {
    const parsed = JSON.parse(trimmed) as { message?: unknown }
    return typeof parsed.message === 'string' && parsed.message.trim() ? parsed.message : reason
  } catch {
    return reason
  }
}

function splitTerms(raw: string): string[] {
  return raw
    .split(/[\n,，]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function sourceLabel(source: ToolSourceType): string {
  if (source === 'keyword') return '关键词'
  if (source === 'topic') return 'Topic'
  return '手动'
}

function formatToolError(err: unknown): string {
  if (err instanceof ToolApiError) {
    if (err.className === 'missing_cooper_url') return '完成测评前需要先关联 Cooper 文档。'
    if (err.className === 'invalid_cooper_url') return 'Cooper 链接必须来自 cooper.didichuxing.com。'
    if (err.className === 'status_conflict' && err.message.includes('already running')) return '正在运行，请等待本次执行完成。'
    if (err.className === 'status_conflict') return '当前状态已变化，请刷新后再操作。'
    if (err.className === 'invalid_github_url') return '请输入有效的 GitHub 仓库地址。'
    if (err.className === 'validation_error' && err.message.includes('final_summary')) return '团队使用说明太长，请缩短后再提交。'
    if (err.className === 'validation_error' && err.message.includes('actor')) return '请先填写操作人。'
    return err.message
  }
  if (err instanceof Error) return err.message
  return '操作失败，请稍后重试。'
}

function formatShortDateTime(iso: string | undefined): string {
  const formatted = formatBeijingTime(iso)
  if (!formatted || formatted === iso) return formatted
  return formatted.slice(5)
}

function formatMonthDay(iso: string | undefined): string {
  const formatted = formatShortDateTime(iso)
  return formatted ? formatted.slice(0, 5).replace('-', '/') : '暂无'
}

function formatPushedAt(iso: string | undefined): string {
  const formatted = formatShortDateTime(iso)
  return formatted || '暂无'
}

function formatGrowth(value: number | undefined): string {
  if (value == null) return '暂无'
  if (value === 0) return '0'
  return `${value > 0 ? '+' : ''}${formatCompactNumber(value)}`
}

function formatCompactNumber(value: number): string {
  const abs = Math.abs(value)
  if (abs >= 1_000_000) return `${trimNumber(value / 1_000_000)}m`
  if (abs >= 1_000) return `${trimNumber(value / 1_000)}k`
  return String(value)
}

function trimNumber(value: number): string {
  return value.toFixed(1).replace(/\.0$/, '')
}

function initials(name: string): string {
  const parts = name.trim().split(/[\s/_-]+/).filter(Boolean)
  if (parts.length >= 2) return `${parts[0][0]}${parts[1][0]}`.toUpperCase()
  const compact = name.trim().replace(/\s+/g, '')
  return (compact.slice(0, 2) || '?').toUpperCase()
}

function toolColor(name: string): string {
  const colors = ['#24334a', '#6653b8', '#2f7d53', '#a85f39', '#285ee8', '#7a5a20']
  let hash = 0
  for (const ch of name) hash = (hash + ch.charCodeAt(0)) % colors.length
  return colors[hash]
}
