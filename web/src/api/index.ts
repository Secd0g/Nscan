import axios from 'axios'

export const http = axios.create({ baseURL: '/api/v1', timeout: 15000 })

http.interceptors.request.use((config) => {
  const token = localStorage.getItem('nscan_token')
  if (token && config.headers) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

http.interceptors.response.use(
  (r) => r,
  (e) => {
    if (e.response && e.response.status === 401) {
      localStorage.removeItem('nscan_token')
      localStorage.removeItem('nscan_user')
      if (window.location.pathname !== '/login') {
        window.location.href = '/login'
      }
    }
    const msg = e.response?.data?.error || e.message
    return Promise.reject(new Error(msg))
  }
)

// ── 类型定义 ──────────────────────────────────────────────────────────────────

export interface Project {
  id: string
  name: string
  description: string
  scope: string[]
  created_at: string
  updated_at: string
}

export interface TaskConfig {
  stages: string[]
  params: Record<string, string>
}

export interface StagePlugin {
  plugin_id: string
  name: string
  enabled: boolean
  params: Record<string, any>
}

export interface Task {
  id: string
  project_id: string
  name: string
  template_id?: string
  template_name?: string
  targets: string[]
  config: TaskConfig
  modules?: Record<string, StagePlugin[]>
  status: 'pending' | 'queued' | 'dispatched' | 'running' | 'done' | 'failed'
  progress?: { stage: string; percent: number; message: string }
  node_id: string
  node_ids?: string[]
  retries: number
  error: string
  created_at: string
  updated_at: string
  started_at?: string
  done_at?: string
  ai_analysis_enabled?: boolean
  ai_analysis_status?: 'pending' | 'running' | 'done' | 'failed'
  ai_analysis?: string
  ai_analysis_error?: string
  ai_analysis_log?: string[]
  ai_analyzed_at?: string
  ai_pentest_enabled?: boolean
  ai_pentest_status?: 'running' | 'done' | 'failed' | 'cancelled'
  ai_pentest_output?: string
  ai_pentest_error?: string
  ai_pentest_log?: string[]
  ai_pentest_node_id?: string
}

export interface Node {
  id: string
  name: string
  addr: string
  status: string
  capabilities: string[]
  installed_tools: string[]
  active_tasks: number
  max_tasks: number
  cpu_percent: number
  mem_percent: number
  version: string
  last_seen: string
  last_seen_at: string
  registered_at: string
}

export interface SubdomainAsset {
  id: string
  task_id: string
  project_id: string
  domain: string
  dns_type: string        // A / CNAME / NS / TXT / MX
  value: string[]         // 解析结果
  ips: string[]
  source: string
  tags: string[]
  created_at: string
}

export interface PortAsset {
  id: string
  task_id: string
  project_id: string
  ip: string
  port: number
  protocol: string
  state: string
  service: string
  banner: string
  products: string[]
  created_at: string
}

export interface HTTPAsset {
  id: string
  task_id: string
  project_id: string
  url: string
  domain: string
  ip: string
  port: number
  status_code: number
  title: string
  tech: string[]
  banner: string
  content_len: number
  server?: string
  tags: string[]
  screenshot?: string
  created_at: string
}

export interface IPAssetFlat {
  ip: string
  ipRowSpan: number
  port: number
  portRowSpan: number
  domain: string
  service: string
  webServer: string
  products: string[]
  time: string
}

export interface StatItem { value: string; count: number }
export interface AssetStats {
  ports: StatItem[]
  techs: StatItem[]
}

export type VulnStatus = 1 | 2 | 3 | 4 | 5 | 6  // 待处理/处理中/忽略/疑似/确认/已处理

export interface VulnAsset {
  id: string
  task_id: string
  project_id: string
  target: string
  template_id: string
  name: string
  severity: 'critical' | 'high' | 'medium' | 'low' | 'info' | 'unknown'
  matched_at: string
  request?: string
  response?: string
  status: VulnStatus
  created_at: string
}

export interface PageResult<T> {
  data: T[]
  total: number
}

// ── Projects ─────────────────────────────────────────────────────────────────

export const projectApi = {
  list: (params?: { limit?: number; skip?: number }) =>
    http.get<PageResult<Project>>('/projects', { params }).then((r) => r.data),
  create: (body: { name: string; description?: string; scope?: string[] }) =>
    http.post<Project>('/projects', body).then((r) => r.data),
  get: (id: string) =>
    http.get<Project>(`/projects/${id}`).then((r) => r.data),
  update: (id: string, body: Partial<Pick<Project, 'name' | 'description' | 'scope'>>) =>
    http.put(`/projects/${id}`, body).then((r) => r.data),
  remove: (id: string) =>
    http.delete(`/projects/${id}`).then((r) => r.data),
  batchRemove: (ids: string[]) =>
    http.delete('/projects', { data: { ids } }).then((r) => r.data),
}

// ── Tasks ─────────────────────────────────────────────────────────────────────

export const taskApi = {
  list: (params?: { project_id?: string; status?: string; keyword?: string; limit?: number; skip?: number }) =>
    http.get<PageResult<Task>>('/tasks', { params }).then((r) => r.data),
  create: (body: {
    project_id: string
    name: string
    targets: string[]
    stages?: string[]
    modules?: Record<string, StagePlugin[]>
    params?: Record<string, string>
    template_id?: string
    template_name?: string
    node_ids?: string[]
    ai_analysis_enabled?: boolean
  }) => http.post<Task>('/tasks', body).then((r) => r.data as Task),
  get: (id: string) =>
    http.get<Task>(`/tasks/${id}`).then((r) => r.data),
  update: (id: string, body: { name?: string; targets?: string[] }) =>
    http.put<Task>(`/tasks/${id}`, body).then((r) => r.data),
  remove: (id: string, withAssets = false) =>
    http.delete(`/tasks/${id}`, { params: withAssets ? { with_assets: 'true' } : {} }).then((r) => r.data),
  batchRemove: (ids: string[], withAssets = false) =>
    http.delete('/tasks', { data: { ids, with_assets: withAssets } }).then((r) => r.data),
  rescan: (id: string) =>
    http.post<Task>(`/tasks/${id}/rescan`).then((r) => r.data),
  cancel: (id: string) =>
    http.post(`/tasks/${id}/cancel`).then((r) => r.data),
  getLogs: (id: string) =>
    http.get<{ data: ProgressEvent[] }>(`/tasks/${id}/logs`).then((r) => r.data.data ?? []),
  getSubtasks: (id: string) =>
    http.get<{ data: Subtask[] }>(`/tasks/${id}/subtasks`).then((r) => r.data.data ?? []),
  getDeadLetter: (id: string) =>
    http.get<{ data: Subtask[] }>(`/tasks/${id}/dead-letter`).then((r) => r.data.data ?? []),
  retryDeadLetter: (subtaskId: string) =>
    http.post(`/dead-letter/${subtaskId}/retry`).then((r) => r.data),
  analyze: (id: string) => http.post<Task>(`/tasks/${id}/ai-analysis`).then((r) => r.data),
  stopAnalyze: (id: string) => http.post(`/tasks/${id}/ai-analysis/stop`).then((r) => r.data),
  startAIPentest: (id: string, body?: { node_id?: string; prompt?: string; timeout_seconds?: number }) => http.post<Task>(`/tasks/${id}/ai-pentest`, body ?? {}).then((r) => r.data),
  stopAIPentest: (id: string) => http.post(`/tasks/${id}/ai-pentest/stop`).then((r) => r.data),
}

export interface Subtask {
  id: string
  task_id: string
  stage: string
  capability: string
  targets: string[]
  attempt: number
  status: 'pending' | 'leased' | 'done' | 'failed' | 'dead_letter'
  leased_by: string
  error_msg?: string
  created_at: string
}

// ── Plugins ─────────────────────────────────────────────────────────────────

export interface ParamOption {
  value: any
  label: string
}

export interface PluginParam {
  key: string
  label: string
  type: 'string' | 'text' | 'number' | 'select' | 'checkbox-group' | 'textarea' | 'switch' | 'dict-select'
  multiple?: boolean
  default?: any
  options?: ParamOption[]
  placeholder?: string
  help?: string
  min?: number
  max?: number
  step?: number
  required?: boolean
  group?: string
  span?: number
  // dict-select 类型下的过滤条件（按字典元数据筛选下拉候选）
  dict_category?: string
  dict_service?: string
  dict_kind?: string
}

export interface Plugin {
  id: string
  name: string
  module: string
  description: string
  version: string
  author: string
  params: PluginParam[]
  builtin: boolean
  enabled: boolean
  source_code?: string
  manifest_json?: string
  category?: string
  icon?: string
  created_at: string
  updated_at: string
}

export const pluginApi = {
  list: (module?: string) =>
    http.get<{ data: Plugin[] }>('/plugins', { params: module ? { module } : {} }).then((r) => r.data.data),
  get: (id: string) =>
    http.get<Plugin>(`/plugins/${id}`).then((r) => r.data),
  create: (body: Partial<Plugin>) =>
    http.post<Plugin>('/plugins', body).then((r) => r.data),
  update: (id: string, body: Partial<Plugin>) =>
    http.put(`/plugins/${id}`, body).then((r) => r.data),
  remove: (id: string) =>
    http.delete(`/plugins/${id}`).then((r) => r.data),
}

// ── Settings (Provider API Keys) ────────────────────────────────────────────

export interface ProviderConfig {
  id: string
  key: string
  providers: Record<string, string[]>
  enabled: Record<string, boolean>
  updated_at: string
}

// ── Sensitive Rules ──────────────────────────────────────────────────────────

export interface SensitiveRule {
  id: string
  name: string
  pattern: string
  color?: string
  description: string
  severity: string
  builtin: boolean
  active: boolean
  created_at: string
  updated_at: string
}

export interface FieldChange {
  field: string
  old: string
  new: string
}
export interface AssetChangeLog {
  id: string
  asset_id: string
  asset_type: string
  asset_label?: string
  project_id: string
  task_id: string
  changes: FieldChange[]
  created_at: string
}

export interface SensitiveAsset {
  id: string
  task_id: string
  project_id: string
  url: string
  rule_id: string
  rule_name: string
  severity: string
  matched: string
  context?: string
  created_at: string
  updated_at: string
}

export const sensitiveRuleApi = {
  list: (params?: { keyword?: string; severity?: string; active?: string; limit?: number; skip?: number }) =>
    http.get<{ data: SensitiveRule[]; total: number }>('/sensitive-rules', { params }).then((r) => r.data),
  create: (body: Partial<SensitiveRule>) =>
    http.post<SensitiveRule>('/sensitive-rules', body).then((r) => r.data),
  update: (id: string, body: Partial<SensitiveRule>) =>
    http.put(`/sensitive-rules/${id}`, body).then((r) => r.data),
  remove: (id: string) =>
    http.delete(`/sensitive-rules/${id}`).then((r) => r.data),
}

export const settingsApi = {
  getProviders: (key: string) =>
    http.get<ProviderConfig>(`/settings/providers/${key}`).then((r) => r.data),
  saveProviders: (key: string, providers: Record<string, string[]>, enabled: Record<string, boolean>) =>
    http.put(`/settings/providers/${key}`, { providers, enabled }).then((r) => r.data),
  getAI: () => http.get<AIConfig>('/settings/ai').then((r) => r.data),
  saveAI: (body: AIConfig) => http.put('/settings/ai', body).then((r) => r.data),
}

export interface AIConfig { type: 'openai' | 'gemini' | 'anthropic' | string; base_url: string; token: string; model: string; proxy_url?: string }

// ── Online Search（Fofa/Hunter/Quake/Shodan 资产查询）────────────────────────

export interface OnlineSearchResult {
  ip: string
  port: number
  host?: string
  url?: string
  title?: string
  server?: string
  country?: string
  region?: string
  city?: string
  protocol?: string
  cert?: string
  banner?: string
  os?: string
  provider: string
}

export const onlineSearchApi = {
  query: (provider: string, query: string, page: number, size: number) =>
    http.post<{ total: number; page: number; size: number; results: OnlineSearchResult[] }>(
      `/online-search/${provider}`,
      { query, page, size },
    ).then((r) => r.data),
  import: (provider: string, projectId: string, results: OnlineSearchResult[]) =>
    http.post<{ imported: number; skipped: number }>(
      `/online-search/${provider}/import`,
      { project_id: projectId, results },
    ).then((r) => r.data),
}

// ── Scan Templates ──────────────────────────────────────────────────────────

export interface ScanTemplate {
  id: string
  name: string
  description: string
  modules: Record<string, StagePlugin[]>
  created_at: string
  updated_at: string
}

export const scanTemplateApi = {
  list: (params?: { limit?: number; skip?: number }) =>
    http.get<PageResult<ScanTemplate>>('/scan-templates', { params }).then((r) => r.data),
  create: (body: { name: string; description?: string; modules: Record<string, StagePlugin[]> }) =>
    http.post<ScanTemplate>('/scan-templates', body).then((r) => r.data),
  update: (id: string, body: Partial<Pick<ScanTemplate, 'name' | 'description' | 'modules'>>) =>
    http.put(`/scan-templates/${id}`, body).then((r) => r.data),
  remove: (id: string) =>
    http.delete(`/scan-templates/${id}`).then((r) => r.data),
  batchRemove: (ids: string[]) =>
    http.delete('/scan-templates', { data: { ids } }).then((r) => r.data),
}

// ── Scheduled（定时扫描）─────────────────────────────────────────────────────

export interface ScheduledJob {
  id: string
  name: string
  project_id: string
  project_name: string
  cron: string
  targets: string[]
  stages: string[]
  params: Record<string, string>
  modules?: Record<string, StagePlugin[]>
  template_id?: string
  template_name?: string
  node_ids?: string[]
  enabled: boolean
  last_run?: string | null
  next_run?: string | null
  run_count: number
  created_at: string
  updated_at: string
}

export interface ScheduledInput {
  name: string
  project_id: string
  cron: string
  targets: string[]
  stages?: string[]
  params?: Record<string, string>
  modules?: Record<string, StagePlugin[]>
  template_id?: string
  template_name?: string
  node_ids?: string[]
  enabled?: boolean
}

export const scheduledApi = {
  list: (params?: { limit?: number; skip?: number }) =>
    http.get<PageResult<ScheduledJob>>('/scheduled', { params }).then((r) => r.data),
  create: (body: ScheduledInput) =>
    http.post<ScheduledJob>('/scheduled', body).then((r) => r.data),
  update: (id: string, body: Partial<ScheduledInput>) =>
    http.put<ScheduledJob>(`/scheduled/${id}`, body).then((r) => r.data),
  remove: (id: string) =>
    http.delete(`/scheduled/${id}`).then((r) => r.data),
  runNow: (id: string) =>
    http.post<{ task_id: string }>(`/scheduled/${id}/run`, {}).then((r) => r.data),
}

// ── Notify（通知设置）────────────────────────────────────────────────────────

export interface NotifyChannel {
  key: string
  enabled: boolean
  events: string[]
  config: Record<string, string>
}

export const notifyApi = {
  list: () =>
    http.get<{ data: NotifyChannel[] }>('/notify').then((r) => r.data.data),
  save: (key: string, body: { enabled: boolean; events: string[]; config: Record<string, string> }) =>
    http.put(`/notify/${key}`, body).then((r) => r.data),
  test: (key: string, config: Record<string, string>) =>
    http.post<{ message: string }>(`/notify/${key}/test`, { config }).then((r) => r.data),
}

// ── WebSocket 进度 ────────────────────────────────────────────────────────────

export interface ProgressEvent {
  task_id: string
  kind: 'progress' | 'status' | 'log'
  stage?: string
  percent?: number
  message?: string
  status?: string
  log?: string
  level?: 'info' | 'warn' | 'error' | 'debug'
}

export function subscribeTaskProgress(
  taskId: string,
  onEvent: (e: ProgressEvent) => void,
  onClose?: () => void,
): WebSocket {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  const token = localStorage.getItem('nscan_token') || ''
  const ws = new WebSocket(`${proto}://${location.host}/ws/tasks/${taskId}/progress?token=${token}`)
  ws.onmessage = (ev) => {
    try { onEvent(JSON.parse(ev.data) as ProgressEvent) } catch {}
  }
  ws.onclose = () => onClose?.()
  return ws
}

// ── Nodes ─────────────────────────────────────────────────────────────────────

export const authApi = {
  captcha: () => http.get<{ captcha_id: string; image: string }>('/auth/captcha').then((r) => r.data),
  login: (data: { username: string; password: string; captcha_id: string; captcha: string }) => http.post('/auth/login', data),
  changePassword: (data: { old_password: string; new_password: string; confirm_password: string }) => http.post('/auth/change-password', data).then((r) => r.data),
}

export const nodeApi = {
  list: () =>
    http.get<{ data: Node[] }>('/nodes').then((r) => r.data.data),
  remove: (id: string) =>
    http.delete(`/nodes/${id}`).then((r) => r.data),
  restart: (id: string) =>
    http.post(`/nodes/${id}/restart`).then((r) => r.data),
  installTool: (id: string, toolName: string, installCmd: string, reinstall = false) =>
    http.post(`/nodes/${id}/install-tool`, { tool_name: toolName, install_cmd: installCmd, reinstall }).then((r) => r.data),
  uninstallTool: (id: string, toolName: string) =>
    http.post(`/nodes/${id}/uninstall-tool`, { tool_name: toolName }).then((r) => r.data),
  token: () =>
    http.get<{ token: string }>('/nodes/token').then((r) => r.data.token),
  regenerateToken: () =>
    http.post<{ token: string }>('/nodes/token/regenerate').then((r) => r.data.token),
}

// ── Tool Definitions ────────────────────────────────────────────────────────

export interface ToolDef {
  name: string
  description: string
  module: string
  install_cmds: string[]
}

export const toolDefApi = {
  list: () =>
    http.get<ToolDef[]>('/tool-defs').then((r) => r.data),
}

// ── Assets ───────────────────────────────────────────────────────────────────

type AssetQuery = {
  task_id?: string
  project_id?: string
  q?: string          // 关键词搜索
  severity?: string   // 漏洞等级过滤
  status?: number     // 漏洞处理状态
  dns_type?: string   // 子域名解析类型
  sort_by?: string
  sort_order?: string // ascending | descending
  limit?: number
  skip?: number
}

// ── POC / Nuclei 模板 ────────────────────────────────────────────────────────

export interface NucleiTemplate {
  id: string
  template_id: string
  name: string
  severity: string
  category: string
  tags: string[]
  author: string
  description?: string
  content?: string
}

export interface CustomPoc {
  id: string
  name: string
  template_id: string
  severity: string
  tags: string[]
  author?: string
  description?: string
  content: string
  enabled: boolean
  created_at: string
}

export interface Dict {
  id: string
  name: string
  description?: string
  service?: string
  path_count?: number
  word_count?: number
  is_builtin: boolean
  enabled: boolean
  content?: string
}

export interface TemplateStats {
  total: number
  critical: number
  high: number
  medium: number
  low: number
  info: number
}

export const pocApi = {
  // Nuclei 模板
  templates: (params?: { category?: string; severity?: string; tag?: string; keyword?: string; limit?: number; skip?: number }) =>
    http.get<PageResult<NucleiTemplate>>('/poc/templates', { params }).then((r) => r.data),
  templateStats: () =>
    http.get<TemplateStats>('/poc/templates/stats').then((r) => r.data),
  templateCategories: () =>
    http.get<string[]>('/poc/templates/categories').then((r) => r.data),
  templateContent: (id: string) =>
    http.get<NucleiTemplate>(`/poc/templates/${id}/content`).then((r) => r.data),
  syncTemplates: (formData: FormData) =>
    http.post('/poc/templates/sync', formData, { headers: { 'Content-Type': 'multipart/form-data' } }).then((r) => r.data),
  syncTemplatesOnline: () =>
    http.post('/poc/templates/sync-online', {}, { timeout: 300000 }).then((r) => r.data),
  clearTemplates: () =>
    http.delete('/poc/templates').then((r) => r.data),

  // 自定义 POC
  pocs: (params?: { name?: string; template_id?: string; severity?: string; tag?: string; enabled?: boolean; limit?: number; skip?: number }) =>
    http.get<PageResult<CustomPoc>>('/poc/custom', { params }).then((r) => r.data),
  pocCreate: (body: Omit<CustomPoc, 'id' | 'created_at'>) =>
    http.post<CustomPoc>('/poc/custom', body).then((r) => r.data),
  pocUpdate: (id: string, body: Partial<CustomPoc>) =>
    http.put(`/poc/custom/${id}`, body).then((r) => r.data),
  pocDelete: (id: string) =>
    http.delete(`/poc/custom/${id}`).then((r) => r.data),
  pocClear: () =>
    http.delete('/poc/custom').then((r) => r.data),
  pocImport: (formData: FormData) =>
    http.post('/poc/custom/import', formData, { headers: { 'Content-Type': 'multipart/form-data' } }).then((r) => r.data),
  pocExport: () =>
    http.get('/poc/custom/export', { responseType: 'blob' }).then((r) => r.data),
  pocValidate: (poc_id: string, url: string, is_template?: boolean) =>
    http.post<{ task_id: string }>('/poc/validate', { poc_id, url, is_template }).then((r) => r.data),
  pocValidateResult: (task_id: string) =>
    http.get<{ status: string; matched: boolean; details: string; severity: string; logs: any[] }>(`/poc/validate/${task_id}`).then((r) => r.data),

  // 字典
  dirscanDicts: (params?: { limit?: number; skip?: number }) =>
    http.get<PageResult<Dict>>('/poc/dicts/dirscan', { params }).then((r) => r.data),
  dirscanDictSave: (body: Partial<Dict>) =>
    http.post<Dict>('/poc/dicts/dirscan', body).then((r) => r.data),
  dirscanDictDelete: (id: string) =>
    http.delete(`/poc/dicts/dirscan/${id}`).then((r) => r.data),
  dirscanDictClear: () =>
    http.delete('/poc/dicts/dirscan/custom').then((r) => r.data),

  subdomainDicts: (params?: { limit?: number; skip?: number }) =>
    http.get<PageResult<Dict>>('/poc/dicts/subdomain', { params }).then((r) => r.data),
  subdomainDictSave: (body: Partial<Dict>) =>
    http.post<Dict>('/poc/dicts/subdomain', body).then((r) => r.data),
  subdomainDictDelete: (id: string) =>
    http.delete(`/poc/dicts/subdomain/${id}`).then((r) => r.data),

  weakpassDicts: (params?: { service?: string; limit?: number; skip?: number }) =>
    http.get<PageResult<Dict>>('/poc/dicts/weakpass', { params }).then((r) => r.data),
  weakpassDictSave: (body: Partial<Dict>) =>
    http.post<Dict>('/poc/dicts/weakpass', body).then((r) => r.data),
  weakpassDictDelete: (id: string) =>
    http.delete(`/poc/dicts/weakpass/${id}`).then((r) => r.data),
}

// ── Assets ───────────────────────────────────────────────────────────────────

export const assetApi = {
  subdomains: (q: AssetQuery) =>
    http.get<PageResult<SubdomainAsset>>('/assets/subdomains', { params: q }).then((r) => r.data),
  ports: (q: AssetQuery) =>
    http.get<PageResult<PortAsset>>('/assets/ports', { params: q }).then((r) => r.data),
  ipAggregated: (q: AssetQuery) =>
    http.get<{ data: IPAssetFlat[]; total: number }>('/assets/ip', { params: q }).then((r) => r.data),
  http: (q: AssetQuery) =>
    http.get<PageResult<HTTPAsset>>('/assets/http', { params: q }).then((r) => r.data),
  vulns: (q: AssetQuery) =>
    http.get<PageResult<VulnAsset>>('/assets/vulns', { params: q }).then((r) => r.data),
  vulnDetail: (id: string) =>
    http.get<{ data: VulnAsset }>(`/assets/vulns/${id}`).then((r) => r.data.data),
  crawler: (q: AssetQuery) =>
    http.get<PageResult<any>>('/assets/crawler', { params: q }).then((r) => r.data),
  sensitive: (q: AssetQuery) =>
    http.get<PageResult<SensitiveAsset>>('/assets/sensitive', { params: q }).then((r) => r.data),
  changes: (type: string, id: string) =>
    http.get<{ data: AssetChangeLog[] }>(`/assets/${type}/${id}/changes`).then((r) => r.data),
  updateVulnStatus: (id: string, status: number) =>
    http.patch(`/assets/vulns/${id}/status`, { status }).then((r) => r.data),
  stats: (q: { task_id?: string; project_id?: string }) =>
    http.get<AssetStats>('/assets/stats', { params: q }).then((r) => r.data),
  sensitiveAgg: (q: { task_id?: string; project_id?: string; q?: string }) =>
    http.get<{ data: { rule_name: string; severity: string; count: number }[] }>('/assets/sensitive/aggregation', { params: q }).then((r) => r.data),
  dirs: (q: AssetQuery & { status_codes?: string }) =>
    http.get<PageResult<any>>('/assets/dirs', { params: q }).then((r) => r.data),
  batchDelete: (type: 'http' | 'port' | 'subdomain' | 'vuln' | 'dir' | 'crawler' | 'sensitive', ids: string[]) =>
    http.delete('/assets/batch', { data: { type, ids } }).then((r) => r.data),
  exportAssets: (type: string, q: { task_id?: string; project_id?: string; q?: string }) =>
    http.get('/export/assets', { params: { type, ...q }, responseType: 'blob' }).then((r) => r.data as Blob),
  exportAllAssets: (q: { task_id?: string; project_id?: string }) =>
    http.get('/export/assets/all', { params: q, responseType: 'blob' }).then((r) => r.data as Blob),
  exportTask: (taskId: string) =>
    http.get(`/export/task/${taskId}`, { responseType: 'blob' }).then((r) => r.data as Blob),
  exportAIReport: (taskId: string) =>
    http.get(`/export/task/${taskId}/ai`, { responseType: 'blob' }).then((r) => r.data as Blob),
  dashboardCounts: () =>
    http.get<DashboardCounts>('/assets/dashboard-counts').then((r) => r.data),
  recentChanges: (limit = 20) =>
    http.get<{ data: AssetChangeLog[] }>('/assets/recent-changes', { params: { limit } }).then((r) => r.data),
  vulnSeverityStats: () =>
    http.get<{ data: VulnSeverityStat[] }>('/assets/vuln-severity-stats').then((r) => r.data),
  dailyTrend: (days = 7) =>
    http.get<{ data: DailyTrendItem[] }>('/assets/daily-trend', { params: { days } }).then((r) => r.data),
}

export interface VulnSeverityStat {
  severity: string
  count: number
}

export interface DailyTrendItem {
  date: string
  subdomain: number
  port: number
  http: number
  vuln: number
}

export interface DashboardCounts {
  subdomains: number
  ports: number
  http: number
  vulns: number
  dirs: number
  sensitive: number
}

// ── Blacklist（全局黑名单）────────────────────────────────────────────────────

export interface BlacklistEntry {
  id: string
  type: string
  value: string
  remark: string
  created_at: string
}

export const blacklistApi = {
  list: () =>
    http.get<{ data: BlacklistEntry[] }>('/blacklist').then((r) => r.data),
  add: (body: { type: string; value: string; remark?: string }) =>
    http.post('/blacklist', body).then((r) => r.data),
  batchAdd: (body: { items: { type: string; value: string; remark?: string }[] }) =>
    http.post('/blacklist/batch', body).then((r) => r.data),
  remove: (id: string) =>
    http.delete(`/blacklist/${id}`).then((r) => r.data),
  clear: () =>
    http.delete('/blacklist').then((r) => r.data),
}

// ── Fingerprint（指纹管理）────────────────────────────────────────────────────

export interface FingerprintEntry {
  id: string
  name: string
  category: string
  parent_category: string
  company: string
  match_type: string
  location: string
  keyword: string
  fp_type: string
  enabled: boolean
  builtin: boolean
  created_at: string
}

export const fingerprintApi = {
  list: (params?: { keyword?: string; parent_category?: string; location?: string; fp_type?: string; enabled?: string; limit?: number; skip?: number }) =>
    http.get<PageResult<FingerprintEntry>>('/fingerprints', { params }).then((r) => r.data),
  categories: () =>
    http.get<string[]>('/fingerprints/categories').then((r) => r.data),
  create: (body: Partial<FingerprintEntry>) =>
    http.post<FingerprintEntry>('/fingerprints', body).then((r) => r.data),
  update: (id: string, body: Partial<FingerprintEntry>) =>
    http.put(`/fingerprints/${id}`, body).then((r) => r.data),
  remove: (id: string) =>
    http.delete(`/fingerprints/${id}`).then((r) => r.data),
  clear: () =>
    http.delete('/fingerprints').then((r) => r.data),
  import: (formData: FormData) =>
    http.post('/fingerprints/import', formData, { headers: { 'Content-Type': 'multipart/form-data' }, timeout: 60000 }).then((r) => r.data),
  syncOnline: () =>
    http.post('/fingerprints/sync-online', {}, { timeout: 120000 }).then((r) => r.data),
}

// ── Dict（字典管理）────────────────────────────────────────────────────────

export interface DictEntry {
  id: string
  category: string
  service?: string
  kind?: string
  name: string
  description: string
  count: number
  builtin: boolean
  active: boolean
  created_at: string
}

export interface DictListParams {
  category?: string
  service?: string
  kind?: string
}

export const dictApi = {
  list: (categoryOrParams?: string | DictListParams) => {
    const params: DictListParams = typeof categoryOrParams === 'string'
      ? { category: categoryOrParams }
      : (categoryOrParams || {})
    return http.get<{ data: DictEntry[] }>('/dicts', { params }).then((r) => r.data)
  },
  create: (body: { category: string; service?: string; kind?: string; name: string; description: string; content: string }) =>
    http.post<DictEntry>('/dicts', body).then((r) => r.data),
  update: (id: string, body: Partial<DictEntry>) =>
    http.put(`/dicts/${id}`, body).then((r) => r.data),
  remove: (id: string) =>
    http.delete(`/dicts/${id}`).then((r) => r.data),
  preview: (id: string, params?: { limit?: number; skip?: number }) =>
    http.get<{ lines: string[]; total: number }>(`/dicts/${id}/preview`, { params }).then((r) => r.data),
  getContent: (id: string) =>
    http.get<{ content: string }>(`/dicts/${id}/content`).then((r) => r.data),
  setContent: (id: string, content: string) =>
    http.put<{ ok: boolean; count: number }>(`/dicts/${id}/content`, { content }).then((r) => r.data),
  clear: (category?: string) =>
    http.delete('/dicts', { params: { category } }).then((r) => r.data),
  syncOnline: () =>
    http.post('/dicts/sync-online', {}, { timeout: 120000 }).then((r) => r.data),
}
