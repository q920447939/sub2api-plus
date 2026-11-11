/**
 * [plus] 上游调度台 · 管理端 API。只有管理端，没有用户端接口（菜单即身份）。
 * 页面上的列、区块、表单字段都由 meta 里的注册表生成，这里不出现具体题目名或指标名。
 */
import { apiClient } from '@/api/client'

const BASE = '/plus/admin/dispatch'

export type Level = 'ok' | 'warn' | 'bad' | 'few' | 'none' | 'info'
export type Role = 'gate' | 'score' | 'display'
export type Mode = 'off' | 'observe' | 'manual'

export interface Gate {
  before: 'cut' | 'none' | 'stop'
  before_pct: number
  after: 'stop' | 'cut'
  after_pct: number
}

export interface Dimension {
  id: number
  name: string
  role: Role
  gate: Gate
  sort: number
  enabled: boolean
}

export interface Item {
  id: number
  name: string
  type: string
  dimension_id: number
  display_only: boolean
  params: any
  rule: any
  platforms: string[]
  enabled: boolean
}

export interface Plan {
  id: number
  item_id: number
  platform: string
  models: string[]
  scopes: PlanScope[]
  exclude_ids: number[]
  interval_minutes: number
  timeout_seconds: number
  enabled: boolean
  next_run_at?: string
}

// 计划「测哪些账号」的一条规则；规则种类和参数由后端注册表声明（meta.scopes）
export interface PlanScope {
  type: string
  params?: Record<string, number>
  account_ids?: number[]
}

export interface ScopeField {
  key: string
  kind: 'number' | 'accounts'
  prefix?: string
  suffix?: string
  min?: number
  max?: number
  default?: number
}

export interface ScopeSpec {
  key: string
  name: string
  fields: ScopeField[]
}

export interface PlanCoverRow {
  account_id: number
  model: string
  ok: boolean
  why?: string
  same_as?: number
  shares: Record<string, number>
  matched?: string[]
}

export interface OutcomeSpec {
  key: string
  label: string
  tone: 'ok' | 'bad' | 'few' | 'err' | 'info'
}

export interface TypeSpec {
  key: string
  name: string
  description: string
  active: boolean
  judges: boolean
  comparable: boolean
  supports: string
  judge_desc: string
  outcomes: OutcomeSpec[] | null
  plan_fields: string[] | null
  gallery: boolean
}

export interface MetricSpec {
  key: string
  name: string
  unit: string
  better: 'low' | 'high'
  desc: string
  counted?: boolean // 出问题的请求占比类指标（报错率、限流率）
  rule: any
}

export interface Config {
  mode: Mode
  pref: 'quality' | 'cost'
  bad_factor: number
  warn_factor: number
  min_share: number
  max_move: number
  concurrency: number
  gap_share: number
  fault_runs: number
  restore_hint: number
}

export interface Meta {
  config: Config
  types: TypeSpec[]
  scopes: ScopeSpec[]
  metrics: MetricSpec[]
  matchers: { key: string; name: string }[]
  references: Record<string, string>
  platforms: string[]
  models: Record<string, { model: string; share: number }[]>
  groups: { id: number; name: string; platform: string; account_ids: number[] }[]
  accounts: { id: number; name: string; platform: string; group_ids: number[]; direct: boolean; direct_why: string; base_url: string; serving: boolean }[]
  dimensions: Dimension[]
  items: Item[]
  plans: Plan[]
  fingerprint_library: { known: boolean; models?: string[] }
  last_eval: string
  last_eval_error: string
}

export interface DimResult {
  dimension_id: number
  level: Level
  model?: string
  item_id?: number
  short: string
  long: string
  muted?: boolean
  no_ref?: boolean
}

export interface Adjustment {
  group_id: number
  account_id: number
  action: 'pause' | 'factor'
  factor: number
  source: string
  reason: string
  log_id?: number
  created_at: string
}

export interface SuggestReason {
  dimension_id: number
  dimension: string
  role: Role
  level: Level
  evidence: string
  action: string
}

export interface SuggestRow {
  account_id: number
  before: number
  after: number
  kind: string
  reasons: SuggestReason[]
  action: 'normal' | 'pause' | 'factor'
  factor: number
  settled?: boolean // 现在的调整已经和建议一样严或更严，不用再调
}

export interface Suggestion {
  group_id: number
  basis: string
  requests: number
  rows: SuggestRow[]
  moved: number
  wanted: number
  capped: boolean
  no_receiver: boolean
  summary: string
}

export interface OverviewRow {
  account_id: number
  name: string
  base_url: string
  rate: number
  serving: boolean
  status: string
  also_in: string[] | null
  share: number
  dims: Record<string, DimResult>
  worst: Level
  one_line: string
  adjustment?: Adjustment
  suggest?: SuggestRow
  is_new: boolean
  direct: boolean
}

export interface OverviewGroup {
  id: number
  name: string
  platform: string
  models: Record<string, number>
  requests_24h: number
  rows: OverviewRow[] | null
  suggest_summary: string
}

export interface Overview {
  dimensions: Dimension[]
  groups: OverviewGroup[] | null
  mode: Mode
  counts: Record<string, number>
  faults: number
  coverage_gaps: { dimension: string; accounts: string[] }[] | null
  as_of: string
  adjustments: number
  hidden: number
}

export interface Verdict {
  level: Level
  short: string
  long: string
  value?: number
  ref?: number
  evidence?: Record<string, any>
}

export interface RunBrief {
  id: number
  t: string
  outcome: string
  summary: string
  detail?: Record<string, any>
}

export interface AdjustmentLog {
  id: number
  group_id: number
  account_id: number
  from_action: string
  from_factor: number
  to_action: string
  to_factor: number
  source: string
  reason: string
  snapshot: Record<string, any>
  operator: string
  created_at: string
}

export interface AdjustRequest {
  group_id: number
  account_id: number
  action: 'normal' | 'pause' | 'factor'
  factor: number
  reason?: string
  source?: 'manual' | 'suggestion'
}

export interface AdjustPreview {
  basis: string
  before: Record<string, number>
  predicted: Record<string, number>
  current: Record<string, string>
  affected: string[] | null
  warning?: string
  mode: Mode
}

export interface AuditEntry {
  id: number
  operator: string
  action: string
  target_type: string
  target_id: string
  summary: string
  before?: any
  after?: any
  created_at: string
}

const get = async <T>(url: string, params?: Record<string, any>) => (await apiClient.get<T>(BASE + url, { params })).data
const post = async <T>(url: string, body?: any) => (await apiClient.post<T>(BASE + url, body ?? {})).data
const put = async <T>(url: string, body?: any) => (await apiClient.put<T>(BASE + url, body ?? {})).data
const del = async <T>(url: string) => (await apiClient.delete<T>(BASE + url)).data

export const dispatchApi = {
  meta: () => get<Meta>('/meta'),
  getConfig: () => get<{ config: Config; version: number }>('/config'),
  saveConfig: (config: Config, version: number) => put<{ version: number }>('/config', { config, version }),
  evaluate: () => post<{ at: string; report: { subjects: number; verdicts: number; changed: number } }>('/evaluate'),

  saveDimension: (d: Partial<Dimension>) => (d.id ? put<Dimension>(`/dimensions/${d.id}`, d) : post<Dimension>('/dimensions', d)),
  deleteDimension: (id: number) => del(`/dimensions/${id}`),
  reorderDimensions: (ids: number[]) => put('/dimensions-order', { ids }),

  saveItem: (it: Partial<Item>) => (it.id ? put<Item>(`/items/${it.id}`, it) : post<Item>('/items', it)),
  deleteItem: (id: number) => del(`/items/${id}`),
  tryGrade: (params: any, reply: string) => post<{ got: string; pass: boolean }>('/items/try-grade', { params, reply }),

  savePlan: (p: Partial<Plan>) => (p.id ? put<Plan>(`/plans/${p.id}`, p) : post<Plan>('/plans', p)),
  deletePlan: (id: number) => del(`/plans/${id}`),
  previewPlan: (p: Partial<Plan>) => post<{ rows: PlanCoverRow[]; targets: number; requests_per_day: number }>('/plans/preview', p),
  runPlan: (id: number) => post<{ queued: number }>(`/plans/${id}/run`),

  overview: (platform = '', all = false) => get<Overview>('/overview', { platform, all: all ? 1 : undefined }),
  account: (id: number, group: number, model = '') => get<any>(`/accounts/${id}`, { group, model }),
  checkup: (id: number, body: { item_ids: number[]; models: string[] }) => post<{ queued: number; skipped: string[] | null }>(`/accounts/${id}/checkup`, body),
  compare: (q: { group: number; model: string; item: number; range: string; highlight?: number }) => get<any>('/compare', q),
  runsBoard: (q: Record<string, any>) => get<any>('/runs/board', q),
  runs: (q: Record<string, any>) => get<any[]>('/runs', q),
  run: (id: number) => get<any>(`/runs/${id}`),
  rerun: (id: number) => post<{ run_id: number }>(`/runs/${id}/rerun`),
  events: (state: string) => get<{ events: any[] | null; counts: Record<string, number> }>('/events', { state }),
  setEventState: (id: number, state: string) => post(`/events/${id}/state`, { state }),
  suggestions: () => get<any[]>('/suggestions'),

  adjustments: () => get<any[] | null>('/adjustments'),
  previewAdjust: (r: AdjustRequest) => post<AdjustPreview>('/adjustments/preview', r),
  applyAdjust: (r: AdjustRequest) => post<AdjustmentLog>('/adjustments', r),
  adjustmentLogs: (q: { group?: number; account?: number } = {}) => get<any[] | null>('/adjustments/logs', q),
  effect: (logId: number) => get<any>(`/adjustments/logs/${logId}/effect`),
  badges: () => get<Record<string, { group_id: number; group: string; action: string; factor: number; text: string }[]>>('/adjustments/badges'),

  audit: (q: Record<string, any>) => get<{ items: AuditEntry[] | null; total: number }>('/audit', q)
}
