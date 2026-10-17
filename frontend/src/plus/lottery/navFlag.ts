// [plus] 侧边栏刮刮乐入口的显示开关：跟随活动总开关，而不是上游的在线支付开关。
// 结果在模块内缓存，最多每 60 秒重新请求一次 overview（侧边栏重新计算时触发），
// 这样登录前的 401 或管理员切换开关后无需刷新页面；请求失败按关闭处理，不影响侧边栏。
import { ref } from 'vue'
import { getOverview } from '@/api/lottery'

const REFRESH_MS = 60_000

const enabled = ref(false)
let lastFetch = 0
let inflight = false

export function lotteryNavFlag(): boolean {
  const now = Date.now()
  if (!inflight && now - lastFetch >= REFRESH_MS) {
    inflight = true
    lastFetch = now
    getOverview()
      .then((o) => {
        enabled.value = !!o.enabled
      })
      .catch(() => {
        enabled.value = false
      })
      .finally(() => {
        inflight = false
      })
  }
  return enabled.value
}
