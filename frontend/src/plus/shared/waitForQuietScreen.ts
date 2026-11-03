// [plus] 等待屏幕"安静"：上游公告弹窗、新手引导都没有在显示时才返回 true。
// 以后其他登录后弹窗也复用这个函数，避免和上游弹窗叠在一起。
import { useAnnouncementStore, useOnboardingStore } from '@/stores'

export interface QuietOptions {
  initialDelayMs?: number // 首次检查前的等待（上游公告在新登录后 3s 才拉取）
  stableMs?: number // 需要连续安静多久
  timeoutMs?: number // 超过这个时间仍不安静就放弃
  signal?: AbortSignal
}

const POLL_MS = 300

export function isScreenBusy(): boolean {
  const announcements = useAnnouncementStore()
  const onboarding = useOnboardingStore()
  if (announcements.currentPopup) return true
  if (onboarding.isDriverActive()) return true
  // 兜底：页面上已经有其他模态框
  return !!document.querySelector('[aria-modal="true"]')
}

export function waitForQuietScreen(opts: QuietOptions = {}): Promise<boolean> {
  const { initialDelayMs = 4000, stableMs = 1200, timeoutMs = 60_000, signal } = opts
  return new Promise((resolve) => {
    const start = Date.now()
    let quietSince = 0
    let timer: ReturnType<typeof setTimeout> | null = null

    const finish = (ok: boolean) => {
      if (timer) clearTimeout(timer)
      signal?.removeEventListener('abort', onAbort)
      resolve(ok)
    }
    const onAbort = () => finish(false)
    signal?.addEventListener('abort', onAbort)

    const tick = () => {
      if (signal?.aborted) return finish(false)
      const now = Date.now()
      if (now - start > timeoutMs) return finish(false)
      if (isScreenBusy()) {
        quietSince = 0
      } else {
        if (!quietSince) quietSince = now
        if (now - quietSince >= stableMs) return finish(true)
      }
      timer = setTimeout(tick, POLL_MS)
    }
    timer = setTimeout(tick, initialDelayMs)
  })
}
