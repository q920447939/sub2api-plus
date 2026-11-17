/**
 * [plus] 反馈中心 API（用户端 + 管理端）
 */
import { apiClient } from '@/api/client'

export type Kind = 'issue' | 'suggestion'
export type Status = 'pending' | 'awaiting' | 'resolved' | 'adopted' | 'declined' | 'duplicate'
export type Severity = '' | 'critical' | 'major' | 'minor' | 'invalid'
export type MsgKind = 'supplement' | 'reply' | 'ask' | 'note' | 'event'
export type FieldType = 'text' | 'textarea' | 'select' | 'datetime'

export interface Field {
  key: string
  label: string
  type: FieldType
  options?: string[]
  placeholder?: string
}

export interface Scenario {
  key: string
  kind: Kind
  label: string
  icon: string
  hint: string
  fields?: Field[]
  /** 停用的场景也会返回（用于显示历史反馈的名称），提交时只能选 enabled !== false 的 */
  enabled?: boolean
}

/** 管理端配置里的场景：enabled 必有 */
export type AdminScenario = Scenario & { enabled: boolean }

export interface Meta {
  enabled: boolean
  float_button: boolean
  notice: string
  scenarios: Scenario[]
  max_images: number
  max_image_mb: number
  content_min: number
  content_max: number
  /** 管理员回复后用户多少天没回应会自动结案，0 = 不自动结案 */
  ask_timeout_days: number
}

export interface FeedbackEvent {
  type: 'closed' | 'reopened' | 'reward' | 'reclass' | 'severity' | 'tags' | 'merged_in' | 'follow_up' | 'auto_close'
  status?: Status
  points?: number
  kind?: Kind
  scenario?: string
  severity?: Severity
  tags?: string[]
  ref_id?: number
}

export interface Message {
  id: number
  author_type: 'user' | 'admin' | 'system'
  author_id?: number
  kind: MsgKind
  body: string
  attachments: number[]
  event?: FeedbackEvent
  internal?: boolean
  created_at: string
}

export interface Feedback {
  id: number
  user_id: number
  kind: Kind
  kind_user?: Kind
  scenario: string
  scenario_user?: string
  content: string
  extra: Record<string, string>
  context?: {
    page?: string
    version?: string
    user_agent?: string
    ip?: string
    account?: { balance: number; subscriptions?: string[] }
  }
  attachments: number[]
  status: Status
  severity?: Severity
  tags?: string[]
  close_reason?: string
  merged_into_id?: number
  prev_id?: number
  reward_points: number
  user_unread: boolean
  last_activity_at: string
  created_at: string
  closed_at?: string
}

export interface Related {
  id: number
  kind: Kind
  status: Status
  summary: string
  relation: 'prev' | 'next' | 'merged_in' | 'merged_into'
}

export interface ListItem {
  id: number
  kind: Kind
  scenario: string
  summary: string
  status: Status
  reward_points: number
  user_unread?: boolean
  last_activity_at: string
  created_at: string
  user_id?: number
  email?: string
  severity?: Severity
  tags?: string[]
  impact?: number
  has_attachments?: boolean
}

export interface Page<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface SubmitInput {
  kind: Kind
  scenario: string
  content: string
  extra: Record<string, string>
  attachments: number[]
  prev_id?: number
  page?: string
  version?: string
}

// ─── 用户端 ──────────────────────────────────────────────────────────────

export const userApi = {
  meta: () => apiClient.get<Meta>('/plus/feedback/meta').then((r) => r.data),
  unread: () => apiClient.get<{ count: number }>('/plus/feedback/unread').then((r) => r.data.count),
  list: (params: { page?: number; page_size?: number; state?: '' | 'open' | 'closed'; kind?: Kind | '' }, config?: { signal?: AbortSignal }) =>
    apiClient.get<Page<ListItem>>('/plus/feedback', { params, ...config }).then((r) => r.data),
  submit: (input: SubmitInput) => apiClient.post<{ id: number }>('/plus/feedback', input).then((r) => r.data.id),
  detail: (id: number) =>
    apiClient
      .get<{ feedback: Feedback; messages: Message[]; related: Related[] }>(`/plus/feedback/${id}`)
      .then((r) => r.data),
  supplement: (id: number, body: string, attachments: number[]) =>
    apiClient.post<Message>(`/plus/feedback/${id}/messages`, { body, attachments }).then((r) => r.data)
}

// ─── 管理端 ──────────────────────────────────────────────────────────────

export interface UserCard {
  id: number
  email: string
  username: string
  balance: number
  subscriptions: string[]
  registered_at: string
  feedbacks: number
  accepted: number
  reward_total: number
}

export interface ErrorLog {
  id: number
  created_at: string
  status_code: number
  model: string
  group: string
  platform: string
  path: string
  phase: string
  message: string
}

export interface AdminDetail {
  feedback: Feedback
  messages: Message[]
  related: Related[]
  user: UserCard | null
  errors: ErrorLog[]
  impact: number
}

export interface AdminQuery {
  status?: string
  kind?: Kind | ''
  scenario?: string
  severity?: string
  tag?: string
  content?: string
  user?: string
  id?: string
  /** 只看近 N 天提交的（与看板口径一致） */
  days?: number | string
  page?: number
  page_size?: number
}

export interface CloseRequest {
  verdict: Status
  reason?: string
  reward_points?: number
  message?: string
}

export interface UpdateInput {
  kind?: Kind
  scenario?: string
  severity?: Severity
  tags?: string[]
}

export interface Config {
  enabled: boolean
  float_button: boolean
  notice: { enabled: boolean; markdown: string }
  scenarios: AdminScenario[]
  limits: { daily_max: number; interval_seconds: number; open_max: number }
  attachments: { max_count: number; max_mb: number }
  ask_timeout_days: number
  reward: { default: number; step: number }
}

export interface Stats {
  days: number
  overview: {
    total: number
    issues: number
    suggestions: number
    pending: number
    awaiting: number
    resolved: number
    adopted: number
    avg_close_hours: number
    auto_closed: number
    reward_points: number
  }
  scenarios: { kind: Kind; scenario: string; total: number; open: number; accepted: number; declined: number; reclassified: number }[]
  severity: { scenario: string; severity: Severity; count: number; open: number }[]
  trend: { date: string; issues: number; suggestions: number; closed: number }[]
  users: {
    user_id: number
    email: string
    total: number
    issues: number
    suggestions: number
    accepted: number
    invalid: number
    reward: number
    scenarios: Record<string, number>
  }[]
  pool: { id: number; scenario: string; summary: string; status: Status; severity?: Severity; impact: number; created_at: string }[]
}

const A = '/plus/admin/feedback'

export const adminApi = {
  list: (params: AdminQuery, config?: { signal?: AbortSignal }) =>
    apiClient.get<Page<ListItem> & { counts: Partial<Record<Status, number>> }>(A, { params, ...config }).then((r) => r.data),
  pendingCount: () => apiClient.get<{ count: number }>(`${A}/pending-count`).then((r) => r.data.count),
  tags: () => apiClient.get<string[]>(`${A}/tags`).then((r) => r.data),
  detail: (id: number) =>
    apiClient.get<{ detail: AdminDetail; reward_enabled: boolean }>(`${A}/${id}`).then((r) => r.data),
  message: (id: number, kind: 'reply' | 'note', body: string, attachments: number[]) =>
    apiClient.post<Message>(`${A}/${id}/messages`, { kind, body, attachments }).then((r) => r.data),
  update: (id: number, input: UpdateInput) => apiClient.patch(`${A}/${id}`, input),
  close: (id: number, req: CloseRequest) => apiClient.post(`${A}/${id}/close`, req),
  batch: (input: { ids: number[]; severity?: Severity; add_tag?: string; decline_reason?: string }) =>
    apiClient.post<{ done: number; failed?: Record<string, string> }>(`${A}/batch`, input).then((r) => r.data),
  stats: (days: number) => apiClient.get<Stats>(`${A}/stats`, { params: { days } }).then((r) => r.data),
  config: () =>
    apiClient.get<{ config: Config; version: number; reward_enabled: boolean }>(`${A}/config`).then((r) => r.data),
  saveConfig: (config: Config, version: number) =>
    apiClient.put<{ version: number }>(`${A}/config`, { config, version }).then((r) => r.data.version)
}

// ─── 附件 ──────────────────────────────────────────────────────────────

export async function uploadImage(file: File, admin = false): Promise<number> {
  const form = new FormData()
  form.append('file', file)
  const { data } = await apiClient.post<{ id: number }>(admin ? `${A}/attachments` : '/plus/feedback/attachments', form, {
    headers: { 'Content-Type': 'multipart/form-data' }
  })
  return data.id
}

// 附件要带鉴权头读取，转成 object URL 给 <img> 用；同一张图只取一次
const blobCache = new Map<string, Promise<string>>()

export function imageURL(id: number, admin = false): Promise<string> {
  const key = `${admin ? 'a' : 'u'}:${id}`
  let p = blobCache.get(key)
  if (!p) {
    p = apiClient
      .get<Blob>(admin ? `${A}/attachments/${id}` : `/plus/feedback/attachments/${id}`, { responseType: 'blob' })
      .then((r) => URL.createObjectURL(r.data))
    p.catch(() => blobCache.delete(key))
    blobCache.set(key, p)
  }
  return p
}
