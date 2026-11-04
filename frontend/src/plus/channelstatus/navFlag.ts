// [plus] 侧边栏「渠道状态」入口的显示开关，跟随后端配置的总开关。
// 结果在模块内缓存，最多每 60 秒重新请求一次，请求失败按关闭处理。
import { ref } from 'vue'
import { getMeta } from './api'

const REFRESH_MS = 60_000

const enabled = ref(false)
let lastFetch = 0
let inflight = false

export function channelStatusNavFlag(): boolean {
  const now = Date.now()
  if (!inflight && now - lastFetch >= REFRESH_MS) {
    inflight = true
    lastFetch = now
    getMeta()
      .then((m) => {
        enabled.value = !!m.enabled
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

