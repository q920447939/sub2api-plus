/**
 * [plus] 上游调度台 · 管理端 API。只有管理端，没有用户端接口（菜单即身份）。
 * 检测项的表单、结果分类都由 meta 里的注册表生成，这里不出现具体题目名或指标名。
 */
import { apiClient } from '@/api/client'

const BASE = '/plus/admin/dispatch'

export type Status = 'up' | 'warn' | 'down' | 'unknown'
export type Gran = 'hour' | 'day'

export interface Slot {
  t: number
  n: number
  score: number | null
  status: Status
  err_pct?: number
  ttft?: number
}

export interface Cols {
  score: number | null
  status: Status
  hours: number
  n24: number
  failed: number
  err_pct: number | null
  ttft: number | null
  tps: number | null
  cache: number | null
  cache_read: number
}

export type ProbeState = 'fail' | 'pass' | 'ignored' | 'fault' | 'few' | 'none' | 'info'

export interface ProbeTag {
  item_id: number
  name: string
  state: ProbeState
  models?: string[]
  short?: string
}

export interface Rule {
  account_id: number
  group_id: number
  model: string
  share: number
  hard: boolean
  pct?: number
  record_id?: number
  created_at: string
}

export interface AccountRow {
  id: number
  name: string
  platform: string
  group_ids: number[]
  created_at: string
  is_new: boolean
  last_at: string | null
  last_hour: number | null
  cols: Cols
  bars: Slot[]
  week_ok: number
  week_fail: number
  down_models: string[]
  probes: ProbeTag[]
  rules: Rule[]
  marks: string[]
  needs: boolean
}

export interface Group {
  id: number
  name: string
  platform: string
}

export interface PlatformSum {
  platform: string
  accounts: number
  worst: Status
  cols: Cols
}

export interface Board {
  now: string
  apply: boolean
  platforms: PlatformSum[]
  groups: Group[]
  accounts: AccountRow[]
}

export interface NameMap {
  accounts: Record<string, string>
  groups: Record<string, string>
}

export interface ModelRow {
  model: string
  cols: Cols
  bars: Slot[]
  probes: ProbeTag[]
  rules: Rule[]
}

export interface LaneRow {
  account_id: number
  pct: number
}

export interface GroupRow {
  group_id: number
  name: string
  bars: Slot[]
  score: number | null
  status: Status
  n24: number
  pct: number
  others: LaneRow[]
}

export interface RuleState {
  share: number
  hard: boolean
  pct?: number
}

export interface EffectItem {
  key: string
  label: string
  unit: string
  before: number | null
  after: number | null
  trend: 'worse' | 'better' | 'flat' | 'na'
}

export interface EffectPart {
  group_id?: number
  model?: string
  nb: number
  na: number
  sb: number | null
  sa: number | null
  cols: Record<string, EffectItem>
  bars?: Slot[]
}

export interface Effect {
  fresh: boolean
  enough: boolean
  hours: number
  nb: number
  na: number
  score_before: number | null
  score_after: number | null
  items: EffectItem[]
  shares: { account_id: number; before: number; after: number | null }[]
  groups: EffectPart[]
  models: EffectPart[]
  dead: { account_id: number; models: string[]; n: number }[]
  better: number
  worse: number
}

export interface RecordView {
  id: number
  account_id: number
  model: string
  kind: 'adjust' | 'restore' | 'undo'
  note: string
  groups: { group_id: number; from: RuleState | null; to: RuleState | null }[]
  side: { account_id: number; group_id: number; model: string; from: RuleState | null; to: RuleState | null }[]
  predicted: { group_id: number; now: number; next: number }[]
  undone_at?: string
  operator: string
  created_at: string
  effect: Effect
}

export interface AccountDetail {
  account: AccountRow
  models: ModelRow[]
  quiet: number
  groups: GroupRow[]
  records: RecordView[]
  names: NameMap
  platform: string
}

export interface MetricRow {
  key: string
  name: string
  unit: string
  value: number | null
  peer: number | null
  worse: boolean
}

export interface RunDot {
  id: number
  outcome: string
  summary: string
  at: string
}

export interface ModelDetail {
  hours: number
  metrics: MetricRow[]
  probes: (ProbeTag & { long: string; dots: RunDot[] })[]
  trend: { labels: number[]; mine: Record<string, (number | null)[]>; peers: Record<string, (number | null)[]> }
  peers: { account_id: number; name: string; me: boolean; cols: Cols; bars: Slot[]; probes: ProbeTag[] }[]
}

export interface Verdict {
  level: 'ok' | 'bad' | 'few' | 'fault' | 'info'
  short: string
  long: string
  valid: number
  bad: number
  ignored?: boolean
}

export interface Run {
  id: number
  plan_id: number | null
  item_id: number
  account_id: number
  platform: string
  model: string
  trigger: string
  status: string
  outcome: string
  summary: string
  detail: Record<string, any>
  prompt?: string
  reply?: string
  error: string
  duration_ms: number | null
  input_tokens: number | null
  output_tokens: number | null
  queued_at: string
}

export interface OutcomeSpec {
  key: string
  label: string
  tone: 'ok' | 'bad' | 'few' | 'err' | 'info'
}

export interface RuleField {
  key: string
  min: number
  max: number
  unit?: string
}

export interface TypeSpec {
  key: string
  name: string
  description: string
  judges: boolean
  gallery: boolean
  has_prompt: boolean
  has_answers: boolean
  rule_text: string
  rule_fields: RuleField[]
  default_every: number
  outcomes: OutcomeSpec[]
}

export interface Item {
  id: number
  name: string
  type: string
  params: Record<string, any>
  rule: Record<string, any>
  enabled: boolean
}

export interface Plan {
  id: number
  item_id: number
  platform: string
  models: string[]
  account_ids: number[]
  interval_minutes: number
  timeout_seconds: number
  enabled: boolean
  next_run_at?: string
}

export interface PlanView extends Plan {
  targets: number
}

export interface PlatformModel {
  model: string
  n: number
  share: number
}

export interface ItemView extends Item {
  spec: TypeSpec
  plans: PlanView[]
  runs24: number
  pass24: number
  judged24: number
  tokens24: number
  failing: number
  targets: number
  covered: number | null
  missing: PlatformModel[]
}

export interface PickAccount {
  id: number
  name: string
  serving: boolean
  direct: boolean
  why?: string
  models: string[]
}

export interface Meta {
  types: TypeSpec[]
  metrics: { key: string; name: string; unit: string; better: string }[]
  platforms: string[]
  models: Record<string, PlatformModel[]>
  accounts: Record<string, PickAccount[]>
  library: { known: boolean; models?: string[]; built_at?: string }
}

export interface HealthRules {
  up_min: number
  warn_min: number
  min_sample: number
  history_min_sample: number
  warning_error_rate: number
  critical_error_rate: number
  target_ttft_ms: number
  critical_ttft_ms: number
  warning_cache_rate: number
  critical_cache_rate: number
  error_weight: number
  ttft_weight: number
  cache_weight: number
  ignored_error_categories: string[]
  exclude_user_side_errors: boolean
}

export interface Config {
  detect: boolean
  apply: boolean
  concurrency: number
  idle_minutes: number
  fault_runs: number
  same_as_channel: boolean
  rules: HealthRules
}

export interface ConfigView {
  config: Config
  version: number
  effective_rules: HealthRules
  counts: Partial<Record<Status, number>>
  serving: number
  closed: number
}

export interface CoverRow {
  account_id: number
  model: string
  ok: boolean
  why?: string
  same_as?: number
  idle?: boolean
}

export interface SheetRow {
  group_id: number
  mode: 'normal' | 'share' | 'pause'
  pct: number
  hard: boolean
}

export interface GroupView {
  group_id: number
  total: number
  pct: number
  locked: number
  reach: number
  alone: string[]
  dead: string[]
  kept: string[]
  rows: LaneRow[]
}

export interface Sheet {
  account_id: number
  name: string
  model: string
  also: string[]
  groups: {
    group_id: number
    name: string
    row: SheetRow
    now: GroupView
    next: GroupView
    from: RuleState | null
    to: RuleState | null
    changed: boolean
    only: boolean
  }[]
  changed: boolean
  danger: boolean
  names: NameMap
}

export interface SheetReq {
  account_id: number
  model: string
  also: string[]
  rows: SheetRow[]
  note?: string
}

export interface Changes {
  apply: boolean
  active: (Rule & { operator: string; note: string })[]
  records: RecordView[]
  names: NameMap
}

export interface ProbeSheet {
  item: Item
  spec: TypeSpec
  account: string
  models: {
    model: string
    verdict: Verdict | null
    every: number | null
    ignore: { up_to_run: number; operator: string; created_at: string } | null
    runs: Run[]
  }[]
  untested: string[]
}

export interface RunsRow {
  account_id: number
  account: string
  group_ids: number[]
  model: string
  item_id: number
  item: string
  dots: RunDot[]
  verdict: Verdict | null
  last: RunDot | null
  every: number | null
  thumb?: number
}

export interface RunsBoard {
  kpi: { total: number; outcomes: Record<string, number>; tokens: number; tested: number; targets: number }
  rows: RunsRow[]
  names: NameMap
  no_plan: boolean
}

export interface RunsQuery {
  item_id?: number
  platform?: string
  result?: string
  q?: string
}

export interface AuditEntry {
  id: number
  operator: string
  action: string
  target_type: 'weight' | 'detect' | 'config'
  target_id: string
  summary: string
  created_at: string
}

const get = <T>(url: string, params?: Record<string, unknown>) => apiClient.get<T>(BASE + url, { params }).then((r) => r.data)
const post = <T>(url: string, body?: unknown) => apiClient.post<T>(BASE + url, body ?? {}).then((r) => r.data)
const put = <T>(url: string, body?: unknown) => apiClient.put<T>(BASE + url, body ?? {}).then((r) => r.data)
const del = <T>(url: string) => apiClient.delete<T>(BASE + url).then((r) => r.data)

export const dispatchApi = {
  meta: () => get<Meta>('/meta'),
  config: () => get<ConfigView>('/config'),
  saveConfig: (config: Config, version: number) => put<ConfigView>('/config', { config, version }),

  board: (gran: Gran) => get<Board>('/board', { gran }),
  account: (id: number, gran: Gran) => get<AccountDetail>(`/accounts/${id}`, { gran }),
  model: (id: number, model: string) => get<ModelDetail>(`/accounts/${id}/model`, { model }),
  probe: (id: number, item: number) => get<ProbeSheet>(`/accounts/${id}/probes/${item}`),
  retest: (id: number, item: number, models: string[]) => post<{ queued: number }>(`/accounts/${id}/probes/${item}/retest`, { models }),
  ignore: (id: number, item: number, model: string, on: boolean) => post(`/accounts/${id}/probes/${item}/ignore`, { model, on }),

  sheet: (accountId: number, model = '') => get<Sheet>('/weights/sheet', { account_id: accountId, model }),
  sheetPreview: (req: SheetReq) => post<Sheet>('/weights/preview', req),
  sheetSave: (req: SheetReq) => post<Sheet>('/weights', req),
  changes: (window: string) => get<Changes>('/changes', { window }),
  restore: (accountId: number, groupId: number, model: string) => post('/rules/restore', { account_id: accountId, group_id: groupId, model }),
  undo: (id: number) => post(`/records/${id}/undo`),
  badges: () => get<Record<string, string>>('/badges'),

  runsBoard: (q: RunsQuery) => get<RunsBoard>('/runs/board', q as Record<string, unknown>),
  runsList: (q: RunsQuery, beforeId?: number) => get<Run[]>('/runs', { ...q, before_id: beforeId }),
  run: (id: number) => get<Run>(`/runs/${id}`),

  items: () => get<ItemView[]>('/items'),
  saveItem: (item: Partial<Item>, plans: Partial<Plan>[]) =>
    item.id ? put<{ id: number }>(`/items/${item.id}`, { item, plans }) : post<{ id: number }>('/items', { item, plans }),
  deleteItem: (id: number) => del(`/items/${id}`),
  toggleItem: (id: number, on: boolean) => post(`/items/${id}/toggle`, { on }),
  tryGrade: (params: Record<string, any>, reply: string) => post<{ answer: string; pass: boolean }>('/items/try-grade', { params, reply }),
  previewPlan: (type: string, plan: Partial<Plan>) => post<CoverRow[]>('/plans/preview', { type, plan }),
  runPlan: (id: number) => post<{ queued: number }>(`/plans/${id}/run`),
  togglePlan: (id: number, on: boolean) => post(`/plans/${id}/toggle`, { on }),

  audit: (q: { kind?: string; q?: string; page?: number; page_size?: number }) =>
    get<{ list: AuditEntry[]; total: number; counts: Record<string, number> }>('/audit', q)
}
