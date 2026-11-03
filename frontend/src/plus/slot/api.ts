/**
 * [plus] 积分乐园 · 幸运九宫格 API（用户端 + 管理端）
 */
import { apiClient } from '@/api/client'

export type PrizeType = 'none' | 'points' | 'balance'

export interface PublicTier {
  label: string
  desc?: string
  emoji?: string
  badge?: string
  prize_type: PrizeType
  boosted?: boolean // 当前有连签加成（不下发具体概率）
}

export interface RuleStatus {
  type: string
  text: string
  passed: boolean
}

export interface Boost {
  type: string
  text: string
}

/** 积分获取方式；action = tab:<name> 切换积分乐园标签 / route:<path> 跳转 */
export interface EarnWay {
  key: string
  label: string
  hint?: string
  action: string
}

export interface Paged<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface SlotOverview {
  enabled: boolean
  cost_per_spin: number
  balance: number
  streak: number
  tiers: PublicTier[]
  rules: RuleStatus[]
  boosts: Boost[]
  earn_ways: EarnWay[]
  can_spin: boolean
  blocker?: string
  reason?: string
  retry_after?: number
  spins_today: number
}

export interface SpinResult {
  spin_id: number
  tier_index: number
  prize_type: PrizeType
  prize_points?: number
  prize_balance?: number
  label: string
  desc?: string
  emoji?: string
  redeem_code?: string
  boosted: boolean
  cost: number
  balance: number
  created_at: string
  replayed: boolean
}

export interface HistoryItem {
  id: number
  cost: number
  tier_index: number
  prize_type: PrizeType
  prize_points?: number
  prize_balance?: number
  label?: string
  emoji?: string
  redeem_code?: string
  created_at: string
}

export interface FeedItem {
  display_name: string
  tier_index: number
  label: string
  emoji: string
  created_at: string
}

// ─── 管理端 ───────────────────────────────────────────────────────────────

export type PotKind = 'small' | 'jackpot'

export interface SlotTier {
  prob_bp: number
  prize_type: PrizeType
  prize_points?: number
  prize_balance?: number
  pot?: PotKind | ''
  label: string
  desc?: string
  emoji?: string
  badge?: string
}

export interface RuleConfig {
  type: string
  enabled: boolean
  params: Record<string, unknown>
}

export interface Atmosphere {
  marquee_enabled: boolean
  marquee_size: number
  virtual_enabled: boolean
  virtual_daily_target: number
  virtual_probs: number[]
}

export interface SlotConfig {
  enabled: boolean
  cost_per_spin: number
  points_per_yuan: number
  tiers: SlotTier[]
  pot: { small_pct: number; jackpot_pct: number }
  rules: RuleConfig[]
  atmosphere: Atmosphere
}

export interface ParamMeta {
  key: string
  kind?: string
  label: string
  min: number
  max: number
  step: number
  default: number
}

export interface CatalogEntry {
  type: string
  name: string
  params: ParamMeta[]
}

export interface PotPreview {
  small_inflow: number
  jackpot_inflow: number
  small_expected: number
  jackpot_expected: number
}

export interface PotStat {
  pot: PotKind
  balance: number
  today_inflow: number
  today_payout: number
  total_inflow: number
  total_payout: number
  today_manual: number
}

export interface SpinStat {
  today_spins: number
  today_cost: number
  total_spins: number
  total_cost: number
}

export interface SlotAdminState {
  config: SlotConfig
  version: number
  rule_catalog: CatalogEntry[]
  prize_kinds: { type: PrizeType; name: string }[]
  base: PotPreview
  boosted: PotPreview
  pots: PotStat[]
  spins: SpinStat
}

export interface PotLedgerItem {
  id: number
  pot: PotKind
  delta: number
  balance: number
  reason: string
  spin_id?: number
  operator_id?: number
  note?: string
  created_at: string
}

export interface AdminSpin extends HistoryItem {
  user_id: number
  email: string
  excluded_tiers: number[] | null
  boosted: boolean
  pot?: PotKind
  pot_payout: number
}

export interface AdminFeedItem extends FeedItem {
  id: number
  is_virtual: boolean
}

export async function getSlotStatus(): Promise<{ enabled: boolean }> {
  const { data } = await apiClient.get<{ enabled: boolean }>('/plus/slot/status')
  return data
}

export async function getSlotOverview(): Promise<SlotOverview> {
  const { data } = await apiClient.get<SlotOverview>('/plus/slot/overview')
  return data
}

export async function postSpin(idempotencyKey: string): Promise<SpinResult> {
  const { data } = await apiClient.post<SpinResult>('/plus/slot/spin', null, {
    headers: { 'Idempotency-Key': idempotencyKey }
  })
  return data
}

export async function getSlotHistory(params: { page: number; page_size: number; win?: boolean }): Promise<Paged<HistoryItem>> {
  const { data } = await apiClient.get<Paged<HistoryItem>>('/plus/slot/history', {
    params: { page: params.page, page_size: params.page_size, win: params.win ? 1 : undefined }
  })
  return data
}

export async function getSlotFeed(): Promise<FeedItem[]> {
  const { data } = await apiClient.get<FeedItem[]>('/plus/slot/feed')
  return data ?? []
}

export async function adminGetSlotState(): Promise<SlotAdminState> {
  const { data } = await apiClient.get<SlotAdminState>('/plus/admin/slot/config')
  return data
}

export async function adminSaveSlotConfig(config: SlotConfig, version: number): Promise<number> {
  const { data } = await apiClient.put('/plus/admin/slot/config', { config, version })
  return data.version
}

export async function adminAdjustPot(pot: PotKind, delta: number, note: string): Promise<number> {
  const { data } = await apiClient.post('/plus/admin/slot/pots/adjust', { pot, delta, note })
  return data.balance
}

export async function adminPotLedger(manualOnly = false, limit = 50): Promise<PotLedgerItem[]> {
  const { data } = await apiClient.get<PotLedgerItem[]>('/plus/admin/slot/pots/ledger', {
    params: { manual: manualOnly ? 1 : undefined, limit }
  })
  return data ?? []
}

export interface SpinQuery {
  page: number
  page_size: number
  q?: string
  prize_type?: PrizeType | ''
  date?: string // YYYY-MM-DD
}

export async function adminSpins(query: SpinQuery): Promise<Paged<AdminSpin>> {
  const { data } = await apiClient.get<Paged<AdminSpin>>('/plus/admin/slot/spins', {
    params: { ...query, q: query.q || undefined, prize_type: query.prize_type || undefined, date: query.date || undefined }
  })
  return data
}

export interface SpinDayStat {
  date?: string
  spins: number
  users: number
  cost: number
  payout: number
  prize_points: number
  prize_balance: number
  balance_wins: number
}

export interface TierStat {
  index: number
  label: string
  emoji?: string
  prize_type: PrizeType
  prob_bp: number
  hits: number
  excluded: number
}

export interface SlotStats {
  days: number
  today: SpinDayStat
  range: SpinDayStat
  trend: SpinDayStat[]
  tiers: TierStat[]
  target_rate: number
}

export async function adminSlotStats(days: number): Promise<SlotStats> {
  const { data } = await apiClient.get<SlotStats>('/plus/admin/slot/stats', { params: { days } })
  return data
}

export async function adminFeed(limit = 50): Promise<AdminFeedItem[]> {
  const { data } = await apiClient.get<AdminFeedItem[]>('/plus/admin/slot/feed', { params: { limit } })
  return data ?? []
}

// ─── 工具 ─────────────────────────────────────────────────────────────────

/** 幂等键：8-64 位 [A-Za-z0-9] */
export function newIdempotencyKey(): string {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'
  const arr = crypto.getRandomValues(new Uint8Array(24))
  return Array.from(arr, (b) => chars[b % chars.length]).join('')
}

/** 奖档价值（积分口径），与后端 Config.ValuePoints 一致 */
export function tierValuePoints(t: SlotTier, pointsPerYuan: number): number {
  if (t.prize_type === 'points') return t.prize_points ?? 0
  if (t.prize_type === 'balance') return Math.round((t.prize_balance ?? 0) * pointsPerYuan)
  return 0
}

/** 万分比 → "12.34%" */
export function fmtBP(bp: number): string {
  return `${(bp / 100).toFixed(2).replace(/\.?0+$/, '')}%`
}

/** 九宫格外圈顺时针顺序：奖档下标 → 3×3 网格位置（4 为中心按钮） */
export const RING_TO_GRID = [0, 1, 2, 5, 8, 7, 6, 3] as const
