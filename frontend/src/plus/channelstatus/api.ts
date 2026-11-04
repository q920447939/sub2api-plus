/**
 * [plus] 渠道状态 API（用户端 + 管理端）
 */
import { apiClient } from '@/api/client'

export type Status = 'up' | 'warn' | 'down' | 'unknown'
export type Granularity = 'hour' | 'day' | 'week'

export interface HistoryBucket {
  t: string
  score: number | null
  status: Status
}

export interface GroupView {
  group_id: number
  name: string
  subtitle: string
  status: Status
  score: number | null
  history: HistoryBucket[]
}

export interface PlatformView {
  platform: string
  name: string
  groups: GroupView[]
}

export interface Snapshot {
  granularity: Granularity
  refresh_seconds: number
  platforms: PlatformView[]
}

export interface Meta {
  enabled: boolean
  refresh_seconds: number
  default_granularity: Granularity
}

export interface GroupLayout {
  group_id: number
  visible: boolean
  display_name: string
  subtitle: string
}

export interface PlatformLayout {
  platform: string
  visible: boolean
  tab_name: string
  groups: GroupLayout[]
}

export interface Rules {
  up_min: number
  warn_min: number
  min_sample: number // 当前状态（判定窗口）
  history_min_sample: number // 历史条每一格
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
  exclude_user_side_errors: boolean // 排除用户侧业务限制错误（余额不足、Key 无效等），与运维监控 SLA 一致
}

export type ProbeFormat = '' | 'anthropic' | 'openai_chat' | 'openai_responses' | 'gemini'

export interface GroupProbe {
  group_id: number
  enabled: boolean
  api_key_id: number
  model: string
  format: ProbeFormat
}

export interface Probe {
  enabled: boolean
  endpoint: string
  fresh_minutes: number
  retry_minutes: number
  base_cooldown_minutes: number
  max_cooldown_hours: number
  daily_limit: number
  timeout_seconds: number
  concurrency: number
  groups: GroupProbe[]
}

export interface ProbeKey {
  id: number
  name: string
  masked: string
  group_id: number
}

export interface ProbeState {
  group_id: number
  last_started_at: string | null
  last_checked_at: string | null
  last_ok: boolean | null
  latency_ms: number | null
  message: string
  idle_streak: number
  day: string
  day_count: number
}

export interface ProbeRunResult {
  ok: boolean
  latency_ms: number
  message: string
}

export interface Config {
  enabled: boolean
  auto_show_new_groups: boolean
  refresh_seconds: number
  judge_window_minutes: number
  default_granularity: Granularity
  layout: PlatformLayout[]
  rules: Rules
  probe: Probe
}

export interface GroupInfo {
  id: number
  name: string
  platform: string
}

export interface AdminConfig {
  config: Config
  version: number
  groups: GroupInfo[]
  new_group_ids: number[]
  error_categories: string[]
}

export async function getSnapshot(granularity?: Granularity): Promise<Snapshot> {
  const { data } = await apiClient.get<Snapshot>('/plus/channel-status', {
    params: granularity ? { granularity } : undefined
  })
  return data
}

export async function getMeta(): Promise<Meta> {
  const { data } = await apiClient.get<Meta>('/plus/channel-status/meta')
  return data
}

export async function getAdminConfig(): Promise<AdminConfig> {
  const { data } = await apiClient.get<AdminConfig>('/plus/admin/channel-status/config')
  return data
}

export async function saveAdminConfig(config: Config, version: number): Promise<number> {
  const { data } = await apiClient.put<{ version: number }>('/plus/admin/channel-status/config', {
    config,
    version
  })
  return data.version
}

export async function listProbeKeys(): Promise<ProbeKey[]> {
  const { data } = await apiClient.get<ProbeKey[]>('/plus/admin/channel-status/probe/keys')
  return data
}

export async function listProbeStates(): Promise<ProbeState[]> {
  const { data } = await apiClient.get<ProbeState[]>('/plus/admin/channel-status/probe/states')
  return data
}

// 立即探测：用页面上的草稿（未保存也能测），跳过冷却与每日上限
export async function runProbe(probe: Probe, group: GroupProbe): Promise<ProbeRunResult> {
  const { data } = await apiClient.post<ProbeRunResult>('/plus/admin/channel-status/probe/run', { probe, group })
  return data
}
