// [plus] 侧边栏积分乐园入口的显示开关：签到或幸运九宫格任一开启即显示。
// 结果在模块内缓存，最多每 60 秒重新请求一次，请求失败按关闭处理。
import { ref } from 'vue'
import { getCheckinStatus } from '../checkin/api'
import { getSlotStatus } from '../slot/api'

const REFRESH_MS = 60_000

const enabled = ref(false)
let lastFetch = 0
let inflight = false

export function pointsParkNavFlag(): boolean {
  const now = Date.now()
  if (!inflight && now - lastFetch >= REFRESH_MS) {
    inflight = true
    lastFetch = now
    Promise.all([
      getCheckinStatus().then((s) => !!s.enabled).catch(() => false),
      getSlotStatus().then((s) => !!s.enabled).catch(() => false)
    ])
      .then(([checkin, grid]) => {
        enabled.value = checkin || grid
      })
      .finally(() => {
        inflight = false
      })
  }
  return enabled.value
}
