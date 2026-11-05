// [plus] 侧边栏「模型画廊」入口：跟随后台「对用户开放」开关，对所有人（包括管理员）一致。
// 结果缓存在模块内，最多每 60 秒请求一次；请求失败按关闭处理。
import { ref } from 'vue'
import { getMeta } from './api'

const REFRESH_MS = 60_000

const enabled = ref(false)
export const iqMenuName = ref('模型画廊')
let lastFetch = 0
let inflight = false

export function iqNavFlag(): boolean {
  const now = Date.now()
  if (!inflight && now - lastFetch >= REFRESH_MS) {
    inflight = true
    lastFetch = now
    getMeta()
      .then((m) => {
        enabled.value = !!m.enabled
        if (m.menu_name) iqMenuName.value = m.menu_name
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
