/**
 * [plus] 签到 API（用户端 + 管理端）
 */
import { apiClient } from '@/api/client'

export interface CheckinResult {
  streak: number
  points_granted: number
  balance: number
  created_at: string
}

export interface CheckinStatus {
  enabled: boolean
  today_checked: boolean
  streak: number // 真实连续签到天数（今日已签含今日，未签为截至昨日）
  balance: number
  today_points: number // 今日签到可得 / 已得积分
  today: string // 服务端东八区日期 YYYY-MM-DD
  month: string // month_checked 所属月份 YYYY-MM
  month_checked: string[]
  rewards: number[] // 7 天奖励循环
  week_bonus: number // 第 7 天额外奖励
  popup_enabled: boolean // 登录后未签到是否弹窗（已叠加总开关）
}

export interface TierBoost {
  tier: number // 幸运九宫格奖档下标
  add_bp: number // 增加的概率（万分比），从谢谢惠顾里扣
}

export interface CheckinConfig {
  enabled: boolean
  rewards: [number, number, number, number, number, number, number]
  week_bonus: number
  ip_daily_max: number
  popup_enabled: boolean
  streak_boost: { enabled: boolean; min_streak: number; tiers: TierBoost[] }
}

export async function postCheckin(): Promise<CheckinResult> {
  const { data } = await apiClient.post<CheckinResult>('/plus/checkin')
  return data
}

export async function getCheckinStatus(month?: string): Promise<CheckinStatus> {
  const { data } = await apiClient.get<CheckinStatus>('/plus/checkin/status', {
    params: month ? { month } : undefined
  })
  return data
}

export async function adminGetCheckinConfig(): Promise<{ config: CheckinConfig; version: number }> {
  const { data } = await apiClient.get('/plus/admin/checkin/config')
  return data
}

export async function adminSaveCheckinConfig(config: CheckinConfig, version: number): Promise<number> {
  const { data } = await apiClient.put('/plus/admin/checkin/config', { config, version })
  return data.version
}

// ─── 管理端：签到记录与统计 ─────────────────────────────────────────────

export interface AdminCheckinLog {
  id: number
  user_id: number
  email: string
  checked_date: string
  streak: number
  points_granted: number
  created_at: string
}

export interface CheckinDayStat {
  date: string
  users: number
  points: number
}

export interface CheckinStats {
  today: CheckinDayStat
  yesterday: CheckinDayStat
  streak_dist: { label: string; users: number }[]
  trend: CheckinDayStat[]
}

export async function adminCheckinLogs(params: {
  page: number
  page_size: number
  q?: string
  date?: string
}): Promise<{ items: AdminCheckinLog[]; total: number; page: number; page_size: number }> {
  const { data } = await apiClient.get('/plus/admin/checkin/logs', {
    params: { ...params, q: params.q || undefined, date: params.date || undefined }
  })
  return data
}

export async function adminCheckinStats(days: number): Promise<CheckinStats> {
  const { data } = await apiClient.get<CheckinStats>('/plus/admin/checkin/stats', { params: { days } })
  return data
}
