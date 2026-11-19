/**
 * [plus] 邮箱验证 API（用户端 + 管理端 + 免登录）
 * 响应拦截器会拆掉 {code,data} 外壳，这里拿到的就是 data。
 */
import { apiClient } from '@/api/client'

export type EmailStatus = 'exempt' | 'verified' | 'pending' | 'invalid'

export interface EmailVerifyStatusView {
  status: EmailStatus
  email: string
  reward_eligible: boolean
  reward_claimed: boolean
  reward_points: number
  /** 当前邮箱待验证或无效：登录后弹出验证提醒 */
  remind: boolean
}

export interface VerifyResult {
  status: EmailStatus
  reward_granted: boolean
  reward_points: number
}

export type VerifySource = 'register' | 'bind' | 'oauth' | 'code' | 'invite'

export const SOURCE_LABEL: Record<VerifySource, string> = {
  register: '注册',
  bind: '绑定邮箱',
  oauth: '第三方登录',
  code: '站内验证码',
  invite: '邀请链接'
}

export interface AdminOverviewStats {
  verified: number
  pending: number
  invalid: number
  pending_paid: number
  pending_free: number
  by_source: Partial<Record<VerifySource, number>>
  reward_total: number
}

export interface VerifyRecord {
  id: number
  user_id: number
  username: string
  email: string
  /** 仍是用户当前邮箱；换过邮箱则为 false */
  current: boolean
  source: VerifySource
  reward_points: number
  verified_at: string
}

export interface VerifyConfig {
  reward_points: number
  invite_ttl_days: number
}

export interface Page<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

const U = '/plus/email-verify'
const A = '/plus/admin/email-verify'

// ─── 用户端 ──────────────────────────────────────────────────────────────────

export const getEmailVerifyStatus = async () => (await apiClient.get<EmailVerifyStatusView>(`${U}/status`)).data
export const sendVerifyCode = async () => { await apiClient.post(`${U}/send-code`) }
export const confirmVerifyCode = async (code: string) => (await apiClient.post<VerifyResult>(`${U}/confirm`, { code })).data

/** 免登录：邮件里的一次性链接，token 本身即凭证 */
export const consumeInviteToken = async (token: string) =>
  (await apiClient.post<VerifyResult>(`${U}/invite`, { token })).data

// ─── 管理端 ──────────────────────────────────────────────────────────────────

export const adminVerifyApi = {
  overview: async () => (await apiClient.get<AdminOverviewStats>(`${A}/overview`)).data,
  records: async (params: { page: number; page_size: number; q?: string; source?: string }) =>
    (await apiClient.get<Page<VerifyRecord>>(`${A}/records`, { params })).data,
  getSettings: async () => (await apiClient.get<{ config: VerifyConfig; version: number }>(`${A}/settings`)).data,
  saveSettings: async (config: VerifyConfig, version: number) =>
    (await apiClient.put<{ version: number }>(`${A}/settings`, { config, version })).data,
  sendInvite: async (userId: number) => { await apiClient.post(`${A}/invite/${userId}`) }
}
