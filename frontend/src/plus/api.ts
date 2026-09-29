/**
 * [plus] API client for fork-specific modules.
 *
 * Returns a discriminated union so callers handle "blocked" and "launchable" uniformly.
 */
import { apiClient } from '@/api/client'

export interface LaunchBlockAction {
  label: string
  path: string
}

export interface LaunchBlockReason {
  code: 'no_group_access' | 'no_balance' | 'key_exhausted' | 'key_expired'
  message: string
  action?: LaunchBlockAction
}

export interface LaunchResult {
  url: string
}

export type LaunchOutcome =
  | { kind: 'ok'; url: string }
  | { kind: 'blocked'; reason: LaunchBlockReason }

/**
 * Resolve a plus menu item into a target URL.
 * The backend reads the module configuration itself and injects credentials, so the browser
 * never sees the key. A refused launch returns HTTP 403 with { blocked: true, reason }.
 */
export async function launchMenu(menuId: string): Promise<LaunchOutcome> {
  try {
    const { data } = await apiClient.post<{ url?: string }>('/plus/menu/launch', {
      menu_id: menuId,
    })
    if (data && typeof data.url === 'string' && data.url) {
      return { kind: 'ok', url: data.url }
    }
    return { kind: 'blocked', reason: { code: 'no_balance', message: '无法打开该应用，请稍后重试' } }
  } catch (err) {
    const resp = err as { status?: number; blocked?: boolean; reason?: LaunchBlockReason }
    if (resp?.status === 403 && resp.reason) {
      return { kind: 'blocked', reason: resp.reason }
    }
    return { kind: 'blocked', reason: { code: 'no_balance', message: '无法打开该应用，请稍后重试' } }
  }
}
