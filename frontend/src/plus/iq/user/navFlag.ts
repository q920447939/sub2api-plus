// [plus] 侧边栏「模型画廊」入口：跟随后台「对用户开放」开关，对所有人（包括管理员）一致。
// 菜单名称（后台可改）也在这里统一缓存：侧边栏、页头、浏览器标题、页面内文案都读 iqMenuName。
// 结果缓存在模块内，最多每 60 秒请求一次；请求失败按关闭处理。
import { ref } from 'vue'
import { getMeta } from './api'

const REFRESH_MS = 60_000
export const IQ_DEFAULT_MENU_NAME = '模型画廊'

const enabled = ref(false)
export const iqMenuName = ref(IQ_DEFAULT_MENU_NAME)
let lastFetch = 0
let inflight: Promise<void> | null = null

/** 刷新开关和菜单名；force 为 true 时忽略 60 秒缓存（例如管理员刚保存设置） */
export function refreshIQMeta(force = false): Promise<void> {
  if (inflight) return inflight
  if (!force && Date.now() - lastFetch < REFRESH_MS) return Promise.resolve()
  lastFetch = Date.now()
  inflight = getMeta()
    .then((m) => {
      enabled.value = !!m.enabled
      iqMenuName.value = m.menu_name || IQ_DEFAULT_MENU_NAME
    })
    .catch(() => {
      enabled.value = false
    })
    .finally(() => {
      inflight = null
    })
  return inflight
}

export function iqNavFlag(): boolean {
  void refreshIQMeta()
  return enabled.value
}
