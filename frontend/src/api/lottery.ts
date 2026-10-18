/**
 * [plus] 兑换刮刮乐 API（用户端 + 管理端）
 */
import { apiClient } from '@/api/client'

// ─── 共享配置类型 ─────────────────────────────────────────────────────────

export interface ChanceTier {
  threshold: number
  chances: number
}

export interface PrizeTier {
  min: number
  max: number
  weight: number
}

export interface PrizePool {
  tiers: PrizeTier[]
}

export interface RiskControl {
  floor_amount: number
  single_order_pct: number
  daily_budget: number
  user_daily_max: number
}

export interface VirtualFeed {
  enabled: boolean
  daily_target: number
  base_total_participants: number
  base_total_awarded: number
  leaderboard_enabled: boolean
}

export interface LotteryConfig {
  enabled: boolean
  enabled_at?: string
  promo_max_amount: number
  chance_tiers: ChanceTier[]
  chance_valid_hours: number
  notes_keyword: string
  prize_pool_real: PrizePool
  prize_pool_virtual: PrizePool
  risk_control: RiskControl
  virtual_feed: VirtualFeed
}

// ─── 用户端类型 ───────────────────────────────────────────────────────────

export interface LotteryOverview {
  enabled: boolean
  promo_max_amount: number
  chance_tiers: ChanceTier[]
  remaining: number
  /** 最早一张卡的过期时间 */
  next_expires_at?: string
  chance_valid_hours: number
  participants: number
  total_awarded: number
  leaderboard_enabled: boolean
}

export interface DrawResult {
  draw_id: number
  chance_id: number
  amount: number
  tier_index: number
  degraded?: string
  remaining: number
  replayed: boolean
  created_at: string
}

export interface FeedItem {
  display_name: string
  amount: number
  created_at: string
}

export interface LeaderboardItem {
  display_name: string
  amount: number
}

export interface UserDraw {
  id: number
  amount: number
  created_at: string
}

// ─── 管理端类型 ───────────────────────────────────────────────────────────

export interface TierStatsRow {
  count: number
  amount: number
}

export interface DailyStats {
  day: string
  draw_count: number
  unique_users: number
  total_awarded: number
  unused_chances: number
  expired_chances: number
  granted_chances: number
  redeem_count: number
  redeem_amount: number
  return_rate: number
  tier_distribution: Record<string, TierStatsRow>
}

export interface AdminDrawRecord {
  id: number
  user_id: number
  display_name: string
  chance_id: number
  redeem_code_id: number
  redeem_code: string
  code_value: number
  amount: number
  tier_index: number
  degraded?: string
  created_at: string
}

export interface AdminDrawsPage {
  items: AdminDrawRecord[]
  total: number
  page: number
  page_size: number
}

// ─── 用户端请求 ───────────────────────────────────────────────────────────

export async function getOverview(): Promise<LotteryOverview> {
  const { data } = await apiClient.get<LotteryOverview>('/plus/lottery/overview')
  return data
}

/**
 * 刮一张卡。idempotencyKey 由调用方为“这一次刮卡动作”生成一次：网络重试时复用同一个 key，
 * 后端会返回同一结果（replayed=true），不会重复扣机会或重复发奖。
 */
export async function draw(idempotencyKey: string, chanceId = 0): Promise<DrawResult> {
  const { data } = await apiClient.post<DrawResult>(
    '/plus/lottery/draw',
    { chance_id: chanceId },
    { headers: { 'Idempotency-Key': idempotencyKey } }
  )
  return data
}

export async function getFeed(limit = 30): Promise<FeedItem[]> {
  const { data } = await apiClient.get<FeedItem[]>('/plus/lottery/feed', {
    params: { limit }
  })
  return data ?? []
}

export async function getLeaderboard(limit = 10): Promise<LeaderboardItem[]> {
  const { data } = await apiClient.get<LeaderboardItem[]>('/plus/lottery/leaderboard', { params: { limit } })
  return data ?? []
}

export async function getMyDraws(limit = 50): Promise<UserDraw[]> {
  const { data } = await apiClient.get<UserDraw[]>('/plus/lottery/my-draws', {
    params: { limit }
  })
  return data ?? []
}

// ─── 管理端请求 ───────────────────────────────────────────────────────────

export async function getConfig(): Promise<{
  config: LotteryConfig
  version: number
}> {
  const { data } = await apiClient.get<{
    config: LotteryConfig
    version: number
  }>('/plus/admin/lottery/config')
  return data
}

/** version 为读取时的版本号；被他人改过会返回冲突错误，需要重新加载 */
export async function saveConfig(config: LotteryConfig, version: number): Promise<number> {
  const { data } = await apiClient.put<{ version: number }>('/plus/admin/lottery/config', {
    config,
    version
  })
  return data.version
}

export async function getStats(day: string): Promise<DailyStats> {
  const { data } = await apiClient.get<DailyStats>('/plus/admin/lottery/stats', { params: { day } })
  return data
}

export async function getDraws(params: {
  day: string
  user_id?: number
  page: number
  page_size: number
}): Promise<AdminDrawsPage> {
  const { data } = await apiClient.get<AdminDrawsPage>('/plus/admin/lottery/draws', { params })
  return { ...data, items: data.items ?? [] }
}

// ─── 工具 ─────────────────────────────────────────────────────────────────

export function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
}

export interface RedeemGrant {
  granted: number
  remaining: number
}

/** 兑换成功后查询本次卡密带来的刮刮卡数 */
export async function getRedeemGrant(): Promise<RedeemGrant> {
  const { data } = await apiClient.get<RedeemGrant>('/plus/lottery/redeem-grant')
  return data
}
