import { apiClient } from '../client'

export type AccountRateSource =
  | 'upstream'
  | 'upstream_stale'
  | 'estimated_probe_off'
  | 'estimated_probe_pending'
  | 'estimated_unsupported'

export interface AccountSummaryItem {
  account_id: number
  account_name: string
  platform: string
  account_type: string
  requests: number
  input_tokens: number
  output_tokens: number
  total_tokens: number
  standard_cost: number
  actual_cost: number
  upstream_cost: number
  estimated_cost: number

  rate_source: AccountRateSource
  upstream_rate: number | null
  account_rate: number | null
  rate_sync_enabled: boolean
  rate_mismatch: boolean
  probe_received_at: string | null
  probe_last_error: string
}

export interface AccountSummaryResponse {
  items: AccountSummaryItem[]
  total_standard_cost: number
  total_actual_cost: number
  total_upstream_cost: number
  total_estimated_cost: number
  total_requests: number
  total_tokens: number
}

export interface AccountUsageTrendPoint {
  date: string
  account_id: number
  account_name: string
  platform: string
  requests: number
  tokens: number
  standard_cost: number
  actual_cost: number
  upstream_cost: number
}

export interface AccountModelStat {
  model: string
  requests: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  total_tokens: number
  standard_cost: number
  actual_cost: number
  upstream_cost: number
}

export async function getAccountSummary(params: {
  start_date: string
  end_date: string
  timezone?: string
  limit?: number
}): Promise<AccountSummaryResponse> {
  const { data } = await apiClient.get<AccountSummaryResponse>('/admin/account-usage/summary', { params })
  return data
}

export async function getAccountTrend(params: {
  start_date: string
  end_date: string
  timezone?: string
  granularity?: string
  limit?: number
}): Promise<AccountUsageTrendPoint[]> {
  const { data } = await apiClient.get<AccountUsageTrendPoint[]>('/admin/account-usage/trend', { params })
  return data
}

export async function getAccountModelStats(params: {
  account_id: number
  start_date: string
  end_date: string
  timezone?: string
}): Promise<AccountModelStat[]> {
  const { data } = await apiClient.get<AccountModelStat[]>('/admin/account-usage/model-stats', { params })
  return data
}
