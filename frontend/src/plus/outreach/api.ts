/**
 * [plus] 用户触达 API（管理端 + 用户端 + 免登录）
 * 响应拦截器会拆掉 {code,data} 外壳，这里拿到的就是 data。
 *
 * 目的、变量、权益、指标都由后端注册表声明（/meta 与统计接口），前端只做通用渲染，不写死任何目的。
 */
import { ref } from 'vue'
import { apiClient } from '@/api/client'
import { formatDateTimeToMinute } from '@/utils/format'

export type BatchStatus = 'pending' | 'running' | 'paused' | 'done' | 'cancelled'
export type PickMode = 'all' | 'random' | 'users'
export type BatchScope = 'fresh' | 'again'

export interface Segment {
  recharged: '' | 'yes' | 'no'
  inactive_days: number
  min_balance: number
  registered_days: number
}

export interface BenefitSpec {
  kind: string
  config: Record<string, number>
}

export interface BenefitField {
  key: string
  label: string
  type: 'int'
  min: number
  max: number
  hint?: string
}

export interface VariableDoc {
  name: string
  desc: string
}

export interface BenefitMeta {
  kind: string
  label: string
  fields: BenefitField[]
  variables: VariableDoc[]
}

export interface PurposeMeta {
  key: string
  label: string
  description: string
  category: 'service' | 'marketing'
  state_text: string
  requires_cta: boolean
  allows_benefits: boolean
  allows_landing: boolean
  default_gap_days: number
  variables: VariableDoc[]
}

export interface Meta {
  purposes: PurposeMeta[]
  benefits: BenefitMeta[]
}

// ─── 外壳 / 模板 ────────────────────────────────────────────────────────────

export interface LayoutConfig {
  header: 'none' | 'text' | 'logo'
  header_name: string   // header=text 时显示的文字；空 = 用站点名
  logo_url: string
  header_link: string
  theme_color: string
  style: 'card' | 'plain'
  width: number
  footer_md: string
}

export interface Layout {
  id: number
  name: string
  is_default: boolean
  config: LayoutConfig
  used_by: number
  updated_at: string
}

export interface LayoutInput {
  name: string
  config: LayoutConfig
}

export interface Template {
  id: number
  name: string
  purpose: string
  subject: string
  body_md: string
  layout_id: number | null
  builtin: boolean
  updated_at: string
}

export type TemplateInput = Pick<Template, 'name' | 'purpose' | 'subject' | 'body_md' | 'layout_id'>

// ─── 活动 / 批次 ────────────────────────────────────────────────────────────

export interface Campaign {
  id: number
  name: string
  purpose: string
  template_id: number | null
  layout_id: number | null
  subject: string
  body_md: string
  segment: Segment
  benefits: BenefitSpec[]
  landing_url: string
  /** 附带权益时：发出后多少天内可领（随批次冻结） */
  claim_days: number
  created_at: string
  updated_at: string
}

export interface CampaignDetail extends Campaign {
  rules: string[]
  template_name: string
  layout_name: string
  batch_count: number
}

export interface MetricView {
  key: string
  label: string
  desc: string
  value: number
  primary: boolean
}

export interface CampaignRow extends Campaign {
  batch_count: number
  running: boolean
  sent: number
  primary: MetricView | null
}

export type CampaignInput = Pick<
  Campaign,
  'name' | 'purpose' | 'template_id' | 'layout_id' | 'subject' | 'body_md' | 'segment' | 'benefits' | 'landing_url' | 'claim_days'
>

export interface Batch {
  id: number
  campaign_id: number
  campaign_name: string
  name: string
  status: BatchStatus
  pause_reason: string
  pick_mode: PickMode
  pick_count: number
  scope: BatchScope
  purpose: string
  subject: string
  body_md: string
  landing_url: string
  benefits: BenefitSpec[]
  claim_days: number
  total: number
  started_at: string | null
  finished_at: string | null
  created_at: string
}

/** 批次只看投递结果；效果在活动层面看 */
export interface BatchRow extends Batch {
  delivery: DeliveryStats
}

export interface BatchRequest {
  name: string
  mode: PickMode
  scope: BatchScope
  count: number
  emails: string[]
}

export interface AudiencePreview {
  matched: number
  eligible: number
  /** 不属于目标人群的原因（已从 eligible 扣除） */
  excluded: { invalid: number; wrong_state: number; unsubscribed: number }
  /** 目标人群内的参考数，不从 eligible 扣除 */
  notes: { recent_mail: number; reached: number; queued: number; available_fresh: number; available_again: number }
  rules: string[]
  samples: { id: number; email: string; username: string }[]
}

export interface DeliveryStats {
  total: number
  queued: number
  sent: number
  failed: number
  skipped: number
  bounced: number
  unsub: number
}

/** 活动累计：投递结果 + 按目的挑选的效果指标 */
export interface Stats extends DeliveryStats {
  metrics: MetricView[]
}

export interface RecipientRow {
  id: number
  user_id: number
  email: string
  status: 'queued' | 'sending' | 'sent' | 'failed' | 'skipped'
  bounced: boolean
  error: string
  sent_at: string | null
  clicked_at: string | null
  claimed_at: string | null
  unsub_at: string | null
}

export interface MailPreview {
  email: string
  status: string
  subject: string
  html: string
}

/** 发送设置：只有投递规则。统计口径归目的、领取有效期归活动、验证奖励归邮箱验证 */
export interface MailSettings {
  link_base_url: string
  per_minute: number
  per_day: number
  marketing_gap_days: number
  purpose_gap_days: Record<string, number>
  breaker_percent: number
  breaker_min_samples: number
  batch_retention_days: number
}

export interface RenderRequest {
  purpose: string
  subject: string
  body_md: string
  layout_id?: number | null
  layout?: LayoutConfig
  benefits?: BenefitSpec[]
}

export interface Page<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

const A = '/plus/admin/outreach'
type PageParams = { page: number; page_size: number }

export const adminApi = {
  getSettings: async () => (await apiClient.get<{ config: MailSettings; version: number }>(`${A}/settings`)).data,
  saveSettings: async (config: MailSettings, version: number) =>
    (await apiClient.put<{ version: number }>(`${A}/settings`, { config, version })).data,
  meta: async () => (await apiClient.get<Meta>(`${A}/meta`)).data,
  preview: async (purpose: string, segment: Segment, campaignId = 0) =>
    (await apiClient.post<AudiencePreview>(`${A}/audience/preview`, { purpose, segment, campaign_id: campaignId })).data,
  render: async (req: RenderRequest) => (await apiClient.post<{ subject: string; html: string }>(`${A}/render`, req)).data,

  layouts: async () => (await apiClient.get<Layout[]>(`${A}/layouts`)).data,
  getLayout: async (id: number) => (await apiClient.get<Layout>(`${A}/layouts/${id}`)).data,
  createLayout: async (input: LayoutInput) => (await apiClient.post<Layout>(`${A}/layouts`, input)).data,
  updateLayout: async (id: number, input: LayoutInput) => (await apiClient.put<Layout>(`${A}/layouts/${id}`, input)).data,
  removeLayout: async (id: number) => { await apiClient.delete(`${A}/layouts/${id}`) },
  setDefaultLayout: async (id: number) => { await apiClient.post(`${A}/layouts/${id}/default`) },

  templates: async (purpose = '') => (await apiClient.get<Template[]>(`${A}/templates`, { params: { purpose } })).data,
  getTemplate: async (id: number) => (await apiClient.get<Template>(`${A}/templates/${id}`)).data,
  createTemplate: async (input: TemplateInput) => (await apiClient.post<Template>(`${A}/templates`, input)).data,
  updateTemplate: async (id: number, input: TemplateInput) => (await apiClient.put<Template>(`${A}/templates/${id}`, input)).data,
  removeTemplate: async (id: number) => { await apiClient.delete(`${A}/templates/${id}`) },

  list: async (params: PageParams, signal?: AbortSignal) =>
    (await apiClient.get<Page<CampaignRow>>(`${A}/campaigns`, { params, signal })).data,
  get: async (id: number) => (await apiClient.get<CampaignDetail>(`${A}/campaigns/${id}`)).data,
  create: async (input: CampaignInput) => (await apiClient.post<Campaign>(`${A}/campaigns`, input)).data,
  update: async (id: number, input: CampaignInput) => (await apiClient.put<Campaign>(`${A}/campaigns/${id}`, input)).data,
  duplicate: async (id: number) => (await apiClient.post<Campaign>(`${A}/campaigns/${id}/duplicate`)).data,
  remove: async (id: number) => { await apiClient.delete(`${A}/campaigns/${id}`) },
  test: async (id: number, sampleEmail = '') =>
    (await apiClient.post<{ sent_to: string }>(`${A}/campaigns/${id}/test`, { sample_email: sampleEmail })).data,
  campaignStats: async (id: number) => (await apiClient.get<Stats>(`${A}/campaigns/${id}/stats`)).data,

  listBatches: async (campaignId: number, params: PageParams) =>
    (await apiClient.get<Page<BatchRow>>(`${A}/campaigns/${campaignId}/batches`, { params })).data,
  createBatch: async (campaignId: number, req: BatchRequest) =>
    (await apiClient.post<{ batch: Batch; requested: number; matched: number }>(`${A}/campaigns/${campaignId}/batches`, req)).data,
  getBatch: async (id: number) => (await apiClient.get<Batch>(`${A}/batches/${id}`)).data,
  removeBatch: async (id: number) => { await apiClient.delete(`${A}/batches/${id}`) },
  startBatch: async (id: number) => { await apiClient.post(`${A}/batches/${id}/start`) },
  pauseBatch: async (id: number) => { await apiClient.post(`${A}/batches/${id}/pause`) },
  resumeBatch: async (id: number) => { await apiClient.post(`${A}/batches/${id}/resume`) },
  cancelBatch: async (id: number) => { await apiClient.post(`${A}/batches/${id}/cancel`) },
  batchStats: async (id: number) => (await apiClient.get<DeliveryStats>(`${A}/batches/${id}/stats`)).data,
  recipients: async (id: number, params: PageParams & { status?: string; q?: string }) =>
    (await apiClient.get<Page<RecipientRow>>(`${A}/batches/${id}/recipients`, { params })).data,
  recipientPreview: async (batchId: number, rid: number) =>
    (await apiClient.get<MailPreview>(`${A}/batches/${batchId}/recipients/${rid}/preview`)).data
}

export interface ClaimInfo {
  items: string[]
  claimed: boolean
  expired: boolean
  next: string
}

export const userApi = {
  claimInfo: async (token: string) => (await apiClient.get<ClaimInfo>('/plus/outreach/claim', { params: { token } })).data,
  claim: async (token: string) => (await apiClient.post<{ items: string[] }>('/plus/outreach/claim', { token })).data
}

export const publicApi = {
  unsubscribe: async (token: string) =>
    (await apiClient.post<{ category: string; unsubscribed: boolean }>('/plus/outreach/unsub', { token })).data
}

// ─── 元数据（目的 / 权益）：页面间共享一份 ───────────────────────────────────

const metaCache = ref<Meta | null>(null)
let metaLoading: Promise<Meta> | null = null

/** 目的、权益等由后端注册表声明；同一会话只拉一次 */
export function useOutreachMeta() {
  if (!metaCache.value && !metaLoading) {
    metaLoading = adminApi
      .meta()
      .then((m) => (metaCache.value = m))
      .finally(() => (metaLoading = null))
  }
  const purposeOf = (key: string) => metaCache.value?.purposes.find((p) => p.key === key)
  const purposeLabel = (key: string) => purposeOf(key)?.label ?? key
  const benefitOf = (kind: string) => metaCache.value?.benefits.find((b) => b.kind === kind)
  const benefitText = (b: BenefitSpec) => {
    const m = benefitOf(b.kind)
    if (!m) return b.kind
    return `${m.label} ${m.fields.map((f) => b.config?.[f.key] ?? '').join('/')}`.trim()
  }
  return { meta: metaCache, purposeOf, purposeLabel, benefitOf, benefitText }
}

// ─── 展示用常量 ────────────────────────────────────────────────────────────

export const BATCH_LABEL: Record<BatchStatus, string> = {
  pending: '待执行',
  running: '执行中',
  paused: '已暂停',
  done: '已完成',
  cancelled: '已取消'
}

export const BATCH_CLASS: Record<BatchStatus, string> = {
  pending: 'badge-gray',
  running: 'badge-primary',
  paused: 'badge-warning',
  done: 'badge-success',
  cancelled: 'badge-gray'
}

export const SCOPE_LABEL: Record<BatchScope, string> = { fresh: '只发给尚未收到的人', again: '再发一轮（含已收到的人）' }

export const PICK_LABEL: Record<PickMode, string> = { all: '全部可选的人', random: '随机取 N 人', users: '指定邮箱' }

export const emptySegment = (): Segment => ({ recharged: '', inactive_days: 0, min_balance: 0, registered_days: 0 })

export const defaultLayoutConfig = (): LayoutConfig => ({
  header: 'text',
  header_name: '',
  logo_url: '',
  header_link: '',
  theme_color: '#2563eb',
  style: 'card',
  width: 580,
  footer_md: '© {{site_name}}'
})

/** 常用人群：点一下填好目的和条件 */
export const PRESETS: { key: string; label: string; purpose: string; segment: Segment; hint: string }[] = [
  { key: 'pending_paid', label: '待验证老客', purpose: 'verify_email', segment: { ...emptySegment(), recharged: 'yes' }, hint: '充值过但还没验证邮箱：验证后有奖励，等于第一波召回' },
  { key: 'pending_all', label: '全部待验证用户', purpose: 'verify_email', segment: emptySegment(), hint: '所有还没验证邮箱的用户' },
  { key: 'silent_paid', label: '沉默老客', purpose: 'recall', segment: { ...emptySegment(), recharged: 'yes', inactive_days: 30 }, hint: '充值过，且 30 天没有登录或调用' },
  { key: 'balance_idle', label: '余额沉睡', purpose: 'recall', segment: { ...emptySegment(), min_balance: 0.01, inactive_days: 14 }, hint: '账户还有余额，14 天没有活跃' },
  { key: 'all_verified', label: '全体已验证用户', purpose: 'news', segment: emptySegment(), hint: '资讯简报、新功能通知' }
]

/** 模板里不能直接写 '}}'（Vue 会当成插值结束），统一在脚本里拼 */
export const varToken = (name: string) => `{{${name}}}`
export const CTA = varToken('cta_url')

export function fmtTime(v: string | null | undefined): string {
  return v ? formatDateTimeToMinute(v) : '—'
}

export function pct(n: number, d: number): string {
  return d > 0 ? `${((n / d) * 100).toFixed(1)}%` : '—'
}
