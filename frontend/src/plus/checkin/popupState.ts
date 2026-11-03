// [plus] 签到弹窗的共享状态：顶栏按钮、登录弹窗、签到页共用一份 status。
// "稍后"记在 sessionStorage（本次登录有效），"今日不再弹出"记在 localStorage（按用户 + 服务端日期）。
import { ref } from 'vue'
import { getCheckinStatus, postCheckin, type CheckinResult, type CheckinStatus } from './api'

const LATER_PREFIX = 'plus:checkin-popup:later:'
const MUTE_PREFIX = 'plus:checkin-popup:mute:'

export const checkinStatus = ref<CheckinStatus | null>(null)
export const popupOpen = ref(false)
/** 每次签到成功 +1，签到页据此刷新 */
export const checkinVersion = ref(0)

let inflight: Promise<CheckinStatus | null> | null = null

export function refreshCheckinStatus(): Promise<CheckinStatus | null> {
  if (inflight) return inflight
  inflight = getCheckinStatus()
    .then((st) => {
      checkinStatus.value = st
      return st
    })
    .catch(() => null)
    .finally(() => {
      inflight = null
    })
  return inflight
}

export async function submitCheckin(): Promise<CheckinResult> {
  const res = await postCheckin()
  checkinVersion.value++
  await refreshCheckinStatus()
  return res
}

/** 签到页自己签到后调用，同步顶栏红点 */
export function notifyCheckedIn(): void {
  checkinVersion.value++
  void refreshCheckinStatus()
}

function safeGet(store: Storage, key: string): string | null {
  try {
    return store.getItem(key)
  } catch {
    return null
  }
}

function safeSet(store: Storage, key: string, value: string): void {
  try {
    store.setItem(key, value)
  } catch {
    // 隐私模式等场景写不进去，只影响下次是否弹
  }
}

export function isPopupSuppressed(userId: number, today: string): boolean {
  if (safeGet(sessionStorage, LATER_PREFIX + userId) === '1') return true
  return safeGet(localStorage, MUTE_PREFIX + userId) === today
}

export function dismissForSession(userId: number): void {
  safeSet(sessionStorage, LATER_PREFIX + userId, '1')
}

export function muteForToday(userId: number, today: string): void {
  safeSet(localStorage, MUTE_PREFIX + userId, today)
  dismissForSession(userId)
}

/** 退出登录时清理：下次登录重新计算 */
export function resetCheckinPopupSession(): void {
  checkinStatus.value = null
  popupOpen.value = false
  try {
    for (let i = sessionStorage.length - 1; i >= 0; i--) {
      const k = sessionStorage.key(i)
      if (k?.startsWith(LATER_PREFIX)) sessionStorage.removeItem(k)
    }
  } catch {
    // ignore
  }
}
