export type DiscoveryMethod = 'keyword' | 'topic'
export type TriggerMode = 'manual' | 'weekly'
export type ToolStatus = 'discovered' | 'evaluating' | 'included' | 'excluded'
export type ToolSort = 'latest' | 'stars' | 'stars7d'
export type ToolSourceType = 'keyword' | 'topic' | 'manual'
export type EvaluationResult = 'included' | 'excluded'
export type DiscoveryRunStatus = 'running' | 'paused' | 'succeeded' | 'partial' | 'failed'

export interface ToolApiEnvelope<T> {
  errno: number
  errmsg: string
  data: T
}

export class ToolApiError extends Error {
  errno: number
  status: number
  className?: string

  constructor(message: string, errno: number, status: number, className?: string) {
    super(message)
    this.name = 'ToolApiError'
    this.errno = errno
    this.status = status
    this.className = className
  }
}

export interface DiscoveryConfig {
  id: string
  name: string
  method: DiscoveryMethod
  terms: string[]
  triggerMode: TriggerMode
  enabled: boolean
  lastSuccessAt?: string
  lastRunAt?: string
  lastRunStatus?: DiscoveryRunStatus
  lastResultCount: number
  lastPagesScanned: number
  lastPauseRequested: boolean
  lastFailureReason?: string
  createdBy: string
  updatedBy: string
  createdAt: string
  updatedAt: string
}

export interface ConfigList {
  count: number
  enabledCount: number
  disabledCount: number
  items: DiscoveryConfig[]
}

export interface SaveConfigInput {
  id?: string
  name: string
  method: DiscoveryMethod
  terms: string[]
  triggerMode: TriggerMode
  enabled?: boolean
  actor: string
}

export interface DiscoveryRun {
  id: string
  status: DiscoveryRunStatus
  finishedAt?: string
  resultCount: number
  newCount: number
  updatedCount: number
  skippedCount: number
  pagesScanned: number
  incompleteResults: boolean
  truncated: boolean
  pauseRequested: boolean
  nextQueryIndex: number
  nextPage: number
  errorClass?: string
  errorMessage?: string
  rateLimited: boolean
	rateLimitResetAt?: string
}

export type GitHubTokenStrategy = 'round_robin' | 'fixed' | 'failover'

export interface ToolSettingsTokenPreview {
  index: number
  masked: string
  last4: string
  source?: string
}

export interface ToolRuntimeSettings {
  githubTokenCount: number
  githubTokens: ToolSettingsTokenPreview[]
  defaultGitHubTokenCount: number
  defaultGitHubTokens: ToolSettingsTokenPreview[]
  githubBaseUrl: string
  githubTokenStrategy: GitHubTokenStrategy
  githubActiveTokenIndex: number
  includeDefaultGitHubTokens: boolean
  githubMaxPages: number
  githubPerPage: number
  githubRequestIntervalMs: number
  starSnapshotLimit: number
  updatedBy?: string
  updatedAt?: string
}

export interface SaveToolSettingsInput {
  githubTokens?: string[]
  clearGitHubTokens?: boolean
  githubBaseUrl?: string
  githubTokenStrategy?: GitHubTokenStrategy
  githubActiveTokenIndex?: number
  includeDefaultGitHubTokens?: boolean
  githubMaxPages?: number
  githubPerPage?: number
  githubRequestIntervalMs?: number
  starSnapshotLimit?: number
  actor: string
}

export interface CheckGitHubSettingsInput {
  githubTokens?: string[]
  githubBaseUrl?: string
  includeDefaultGitHubTokens?: boolean
}

export interface GitHubTokenCheck {
  index: number
  masked: string
  last4: string
  source: string
  ok: boolean
  limit?: number
  remaining?: number
  resetAt?: string
  error?: string
}

export interface GitHubRepositoryPreview {
  nodeId: string
  owner: string
  repo: string
  fullName: string
  url: string
  homepageUrl?: string
  name: string
  description?: string
  summary?: string
  stars: number
  forks: number
  openIssues: number
  topics: string[]
  purposeTags: string[]
  licenseSpdx?: string
  defaultBranch?: string
  pushedAt?: string
}

export interface ToolSource {
  sourceType: ToolSourceType
  term: string
  configId?: string
  lastSeenAt: string
  hitCount: number
}

export interface ToolEvaluation {
  id: string
  evaluator: string
  operator: string
  cooperUrl?: string
  startedAt: string
  completedAt?: string
  result?: EvaluationResult
  finalSummary?: string
  notIncludedReason?: string
}

export interface ToolItem {
  id: string
  name: string
  githubNodeId: string
  githubOwner: string
  githubRepo: string
  githubFullName: string
  githubUrl: string
  homepageUrl?: string
  description?: string
  temporarySummary?: string
  temporarySummarySource?: string
  finalSummary?: string
  currentSummary?: string
  currentSummarySource: 'final' | 'temporary' | 'github_description' | 'none'
  stars: number
  forks: number
  openIssues: number
  stars7d?: number
  topics: string[]
  purposeTags: string[]
  purposeTagsManuallySet: boolean
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

export interface ToolListStats {
  status: ToolStatus
  keywordSourceCount: number
  topicSourceCount: number
  manualSourceCount: number
  linkedEvaluationCount: number
  unlinkedEvaluationCount: number
  evaluatorCount: number
  latestUpdatedAt?: string
  purposeTags: string[]
}

export interface ToolList {
  count: number
  page: number
  pageSize: number
  offset: number
  take: number
  stats: ToolListStats
  items: ToolItem[]
}

export interface ListToolsQuery {
  status?: ToolStatus
  sort?: ToolSort
  source?: ToolSourceType
  sources?: ToolSourceType[]
  purposeTags?: string[]
  q?: string
  take?: number
  offset?: number
  page?: number
}

export interface ListConfigQuery {
  q?: string
  methods?: DiscoveryMethod[]
  triggerModes?: TriggerMode[]
}

export interface ToolSummary {
  toolId: string
  urlKey: string
  summary: string
  source: string
  translated: boolean
}

async function requestToolAPI<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: {
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      ...init?.headers,
    },
  })
  const body = await res.json().catch(() => null) as ToolApiEnvelope<T> | null
  if (!res.ok || !body || body.errno !== 0) {
    const data = body?.data as { class?: string } | undefined
    throw new ToolApiError(body?.errmsg || `tools api failed: ${res.status}`, body?.errno ?? res.status, res.status, data?.class)
  }
  return body.data
}

function postToolAPI<T>(path: string, body: unknown): Promise<T> {
  return requestToolAPI<T>(path, { method: 'POST', body: JSON.stringify(body) })
}

export function fetchToolConfigs(query?: string | ListConfigQuery): Promise<ConfigList> {
  const params = new URLSearchParams()
  if (typeof query === 'string') {
    if (query) params.set('q', query)
  } else if (query) {
    if (query.q) params.set('q', query.q)
    for (const method of query.methods ?? []) params.append('method', method)
    for (const triggerMode of query.triggerModes ?? []) params.append('triggerMode', triggerMode)
  }
  const suffix = params.toString()
  return requestToolAPI<ConfigList>(`/api/tools/configs${suffix ? '?' + suffix : ''}`)
}

export async function saveToolConfig(input: SaveConfigInput): Promise<DiscoveryConfig> {
  const data = await postToolAPI<{ config: DiscoveryConfig }>('/api/tools/configs', input)
  return data.config
}

export async function setToolConfigEnabled(id: string, enabled: boolean, actor: string): Promise<DiscoveryConfig> {
  const data = await postToolAPI<{ config: DiscoveryConfig }>('/api/tools/configs/enable', { id, enabled, actor })
  return data.config
}

export async function deleteToolConfig(id: string, actor: string): Promise<boolean> {
  const data = await postToolAPI<{ deleted: boolean }>('/api/tools/configs/delete', { id, actor })
  return data.deleted
}

export async function runToolConfig(id: string, actor: string): Promise<DiscoveryRun> {
  const data = await postToolAPI<{ run: DiscoveryRun }>('/api/tools/configs/run', { id, actor })
  return data.run
}

export async function pauseToolConfig(id: string, actor: string): Promise<DiscoveryRun> {
  const data = await postToolAPI<{ run: DiscoveryRun }>('/api/tools/configs/pause', { id, actor })
  return data.run
}

export async function resumeToolConfig(id: string, actor: string): Promise<DiscoveryRun> {
	const data = await postToolAPI<{ run: DiscoveryRun }>('/api/tools/configs/resume', { id, actor })
	return data.run
}

export async function fetchToolSettings(): Promise<ToolRuntimeSettings> {
  const data = await requestToolAPI<{ settings: ToolRuntimeSettings }>('/api/tools/settings')
  return data.settings
}

export async function saveToolSettings(input: SaveToolSettingsInput): Promise<ToolRuntimeSettings> {
  const data = await postToolAPI<{ settings: ToolRuntimeSettings }>('/api/tools/settings', input)
  return data.settings
}

export async function checkGitHubSettings(input: CheckGitHubSettingsInput): Promise<GitHubTokenCheck[]> {
  const data = await postToolAPI<{ tokens: GitHubTokenCheck[] }>('/api/tools/settings/check-github', input)
  return data.tokens
}

export function fetchTools(query: ListToolsQuery): Promise<ToolList> {
  const params = new URLSearchParams()
  if (query.status) params.set('status', query.status)
  if (query.sort) params.set('sort', query.sort)
  const sources = query.sources ?? (query.source ? [query.source] : [])
  for (const source of sources) params.append('source', source)
  for (const tag of query.purposeTags ?? []) params.append('purposeTag', tag)
  if (query.q) params.set('q', query.q)
  if (query.take != null) params.set('take', String(query.take))
  if (query.offset != null) params.set('offset', String(query.offset))
  if (query.page != null) params.set('page', String(query.page))
  const suffix = params.toString()
  return requestToolAPI<ToolList>(`/api/tools/items${suffix ? '?' + suffix : ''}`)
}

export function fetchToolSummaryByURLKey(input: { toolId: string; urlKey: string }): Promise<ToolSummary> {
  return postToolAPI('/api/tools/summaries/url-key', input)
}

export async function previewManualTool(repoUrl: string): Promise<GitHubRepositoryPreview> {
  const data = await postToolAPI<{ repository: GitHubRepositoryPreview }>('/api/tools/manual/preview', { repoUrl })
  return data.repository
}

export async function addManualTool(repoUrl: string, actor: string, purposeTags?: string[]): Promise<{ tool: ToolItem; created: boolean; duplicate: boolean }> {
  return postToolAPI('/api/tools/manual/add', { repoUrl, actor, purposeTags })
}

export async function importTeamTool(input: {
  repoUrl: string
  operator: string
  cooperUrl?: string
  finalSummary?: string
  purposeTags?: string[]
}): Promise<{ tool: ToolItem; created: boolean; duplicate: boolean }> {
  return postToolAPI('/api/tools/team/import', input)
}

export function updateToolPurposeTags(input: {
  toolId: string
  actor: string
  purposeTags: string[]
}): Promise<{ tool: ToolItem }> {
  return postToolAPI('/api/tools/purpose-tags', input)
}

export function updateTeamTool(input: {
  toolId: string
  operator: string
  cooperUrl?: string
  finalSummary?: string
}): Promise<{ updated: boolean }> {
  return postToolAPI('/api/tools/team/update', input)
}

export function deleteTeamTool(toolId: string, operator: string, reason: string): Promise<{ deleted: boolean }> {
  return postToolAPI('/api/tools/team/delete', { toolId, operator, reason })
}

export async function startToolEvaluation(input: {
  toolId: string
  evaluator: string
  operator: string
  cooperUrl?: string
}): Promise<ToolEvaluation> {
  const data = await postToolAPI<{ evaluation: ToolEvaluation }>('/api/tools/evaluations/start', input)
  return data.evaluation
}

export async function updateToolEvaluationCooperURL(toolId: string, cooperUrl: string, operator: string): Promise<ToolEvaluation> {
  const data = await postToolAPI<{ evaluation: ToolEvaluation }>('/api/tools/evaluations/cooper-url', { toolId, cooperUrl, operator })
  return data.evaluation
}

export function finishToolEvaluation(input: {
  toolId: string
  evaluationId: string
  result: EvaluationResult
  operator: string
  finalSummary?: string
  notIncludedReason?: string
}): Promise<{ completed: boolean }> {
  return postToolAPI('/api/tools/evaluations/finish', input)
}

export function excludeDiscoveredTool(toolId: string, reason: string, operator: string): Promise<{ excluded: boolean }> {
  return postToolAPI('/api/tools/discovered/exclude', { toolId, reason, operator })
}
