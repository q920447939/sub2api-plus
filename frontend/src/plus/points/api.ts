/**
 * [plus] 积分 API（用户端 + 管理端）
 */
import { apiClient } from '@/api/client'

// ─── 用户端类型 ───────────────────────────────────────────────────────────

export interface PointsOverview {
  balance: number
  expire_at?: string       // ISO8601，存在时展示过期提示
  warn_days?: number       // 距清零剩余天数，>0 时展示预警
}

export interface LedgerItem {
  id: number
  delta: number
  balance: number
  reason: string
  ref_id?: number
  note?: string
  created_at: string
}

export interface LedgerPage {
  items: LedgerItem[]
  total: number
  page: number
  page_size: number
}

// ─── 管理端类型 ───────────────────────────────────────────────────────────

export interface RedeemBonusTier {
  threshold: number // 卡密面值满 $X
  points: number // 送 Y 积分
}

export interface PointsConfig {
  expire_days: number
  warn_days: number
  redeem_bonus: {
    enabled: boolean
    enabled_at?: string // 服务端维护：最近一次开启时间
    tiers: RedeemBonusTier[]
  }
}

export interface RedeemGrant {
  code_value: number
  points: number
  created_at: string
}

// ─── 用户端 API ───────────────────────────────────────────────────────────

export async function getPointsOverview(): Promise<PointsOverview> {
  const { data } = await apiClient.get<PointsOverview>('/plus/points/overview')
  return data
}

export async function getPointsLedger(params: {
  page?: number
  page_size?: number
  reason?: string
}): Promise<LedgerPage> {
  const { data } = await apiClient.get<LedgerPage>('/plus/points/ledger', { params })
  return data
}

/** 最近 5 分钟内兑换卡密获得的积分，没有返回 null */
export async function getLatestRedeemBonus(): Promise<RedeemGrant | null> {
  const { data } = await apiClient.get<RedeemGrant | null>('/plus/points/redeem-bonus/latest')
  return data ?? null
}

// ─── 管理端 API ───────────────────────────────────────────────────────────

export async function adminGetPointsConfig(): Promise<{ config: PointsConfig; version: number }> {
  const { data } = await apiClient.get('/plus/admin/points/config')
  return data
}

export async function adminSavePointsConfig(config: PointsConfig, version: number): Promise<number> {
  const { data } = await apiClient.put('/plus/admin/points/config', { config, version })
  return data.version
}

// ─── 管理端：账户 / 流水 / 手动调整 / 统计 ─────────────────────────────────

export interface Paged<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface AdminAccount {
  user_id: number
  email: string
  username?: string
  balance: number
  total_earned: number
  total_spent: number
  last_active_at: string
  expire_at?: string
}

export interface AdminLedgerItem extends LedgerItem {
  user_id: number
  email: string
  operator_id?: number
  operator_email?: string
  remark?: string
}

export interface ReasonAmount {
  reason: string
  amount: number
}

export interface PointsDayStat {
  date: string
  issued: number
  consumed: number
  expired: number
}

export interface PointsStats {
  outstanding: number
  accounts: number
  today_issued: ReasonAmount[]
  today_consumed: ReasonAmount[]
  trend: PointsDayStat[]
  top_holders: TopHolder[] // 剩余积分最多的前 10 名
}

export interface TopHolder {
  rank: number
  user_id: number
  email: string
  username?: string
  balance: number
  last_active_at: string
  expire_at?: string
}

export async function adminListAccounts(params: {
  page: number
  page_size: number
  q?: string
  sort?: 'balance' | 'last_active'
}): Promise<Paged<AdminAccount>> {
  const { data } = await apiClient.get<Paged<AdminAccount>>('/plus/admin/points/accounts', {
    params: { ...params, q: params.q || undefined }
  })
  return data
}

export async function adminPointsLedger(params: {
  page: number
  page_size: number
  user_id?: number
  q?: string
  reason?: string
}): Promise<Paged<AdminLedgerItem>> {
  const { data } = await apiClient.get<Paged<AdminLedgerItem>>('/plus/admin/points/ledger', {
    params: { ...params, q: params.q || undefined, reason: params.reason || undefined }
  })
  return data
}

export async function adminAdjustPoints(userId: number, delta: number, remark: string): Promise<number> {
  const { data } = await apiClient.post('/plus/admin/points/adjust', { user_id: userId, delta, remark })
  return data.balance
}

export async function adminPointsStats(days: number): Promise<PointsStats> {
  const { data } = await apiClient.get<PointsStats>('/plus/admin/points/stats', { params: { days } })
  return data
}

/** 积分流水来源/去向的展示名与图标（用户端与管理端共用） */
export const REASON_META: Record<string, { label: string; icon: string }> = {
  checkin: { label: '每日签到', icon: '📅' },
  redeem_bonus: { label: '兑换卡密赠送', icon: '🎫' },
  slot_consume: { label: '幸运九宫格消耗', icon: '🎰' },
  slot_win: { label: '幸运九宫格奖励', icon: '🎉' },
  expire: { label: '积分过期清零', icon: '⏳' },
  admin_adjust: { label: '积分调整', icon: '🛠️' }
}
export const reasonLabel = (r: string) => REASON_META[r]?.label ?? r
export const reasonIcon = (r: string) => REASON_META[r]?.icon ?? '🪙'
