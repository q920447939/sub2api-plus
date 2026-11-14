// [plus] 调度台调整标签：整页只请求一次（1 分钟内复用），模块没启用或没权限时静默
import { ref } from 'vue'
import { dispatchApi } from './api'

export const badges = ref<Record<string, string>>({})
let loadedAt = 0
let inflight: Promise<void> | null = null

export function loadBadges(): Promise<void> {
  if (Date.now() - loadedAt < 60_000) return Promise.resolve()
  if (!inflight) {
    inflight = dispatchApi
      .badges()
      .then((b) => {
        badges.value = b ?? {}
        loadedAt = Date.now()
      })
      .catch(() => {
        loadedAt = Date.now()
      })
      .finally(() => {
        inflight = null
      })
  }
  return inflight
}
