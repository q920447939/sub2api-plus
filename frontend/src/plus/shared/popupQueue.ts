// [plus] 登录后自动弹窗的排队器：同一时间只显示一个 plus 弹窗，并且只在屏幕"安静"时（上游公告、新手引导、其他模态框都没在显示）才放行。
//
// 为什么需要：每个弹窗各自调用 waitForQuietScreen 时，两个弹窗可能在同一个 tick 都判定"安静"而同时弹出。
// 排队器把"等安静 → 选出优先级最高的申请者 → 交出显示权"串成一条线，显示权归还后再放行下一个。
//
// 用法：
//   const release = await acquirePopupSlot('email-verify', PRIORITY.warning, signal)
//   if (!release) return            // 被取消或长时间不安静
//   open.value = true               // 关闭时调用 release()
import { waitForQuietScreen } from './waitForQuietScreen'

/** 数字越大越先弹 */
export const POPUP_PRIORITY = {
  warning: 100, // 需要用户处理的警告（如邮箱未验证）
  engagement: 10 // 运营类（如每日签到）
} as const

interface Waiter {
  key: string
  priority: number
  seq: number
  signal?: AbortSignal
  resolve: (release: (() => void) | null) => void
}

const waiters: Waiter[] = []
let holder: string | null = null
let pumping = false
let seq = 0

/** 申请显示权。返回 release 函数（弹窗关闭时必须调用）；被取消或超时返回 null。 */
export function acquirePopupSlot(key: string, priority: number, signal?: AbortSignal): Promise<(() => void) | null> {
  return new Promise((resolve) => {
    if (signal?.aborted) return resolve(null)
    const w: Waiter = { key, priority, seq: seq++, signal, resolve }
    waiters.push(w)
    signal?.addEventListener('abort', () => {
      const i = waiters.indexOf(w)
      if (i >= 0) {
        waiters.splice(i, 1)
        resolve(null)
      }
    })
    void pump()
  })
}

async function pump(initialDelayMs?: number): Promise<void> {
  if (pumping || holder) return
  pumping = true
  try {
    while (!holder && waiters.length) {
      // 先等屏幕安静，再在此刻的申请者里挑优先级最高的：晚到但更重要的弹窗也能排到前面
      const quiet = await waitForQuietScreen(initialDelayMs === undefined ? {} : { initialDelayMs })
      if (!quiet) {
        // 长时间不安静（例如用户一直开着公告）：本次登录不再打扰
        waiters.splice(0).forEach((w) => w.resolve(null))
        return
      }
      waiters.sort((a, b) => b.priority - a.priority || a.seq - b.seq)
      const w = waiters.shift()
      if (!w || w.signal?.aborted) continue
      holder = w.key
      let released = false
      w.resolve(() => {
        if (released) return
        released = true
        holder = null
        // 给关闭动画留时间，再放行下一个
        void pump(800)
      })
    }
  } finally {
    pumping = false
  }
}

/** 测试用 */
export function _resetPopupQueue(): void {
  waiters.splice(0)
  holder = null
  pumping = false
}
