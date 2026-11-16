// [plus] 反馈中心的全局状态：配置（meta）、红点计数、提交面板开关。
// 侧边栏、悬浮按钮、页面共用同一份，避免重复请求。
import { reactive, ref } from 'vue'
import { userApi, adminApi, type Meta, type Scenario } from './api'

const REFRESH_MS = 60_000

// ─── meta ──────────────────────────────────────────────────────────────

export const meta = ref<Meta | null>(null)
let metaAt = 0
let metaInflight: Promise<Meta | null> | null = null

export function loadMeta(force = false): Promise<Meta | null> {
  if (!force && meta.value && Date.now() - metaAt < REFRESH_MS) return Promise.resolve(meta.value)
  if (!metaInflight) {
    metaInflight = userApi
      .meta()
      .then((m) => (meta.value = m))
      .catch(() => meta.value)
      .finally(() => {
        metaAt = Date.now()
        metaInflight = null
      })
  }
  return metaInflight
}

/** 侧边栏开关：跟随后台总开关，未加载时先隐藏 */
export function feedbackNavFlag(): boolean {
  void loadMeta()
  return !!meta.value?.enabled
}

export function scenarioOf(key: string, list?: Scenario[]): Scenario | undefined {
  return (list ?? meta.value?.scenarios ?? []).find((s) => s.key === key)
}

// ─── 红点 ──────────────────────────────────────────────────────────────

export const badges = reactive({ unread: 0, pending: 0 })
const loadedAt = { unread: 0, pending: 0 }

function poll(key: 'unread' | 'pending', fetcher: () => Promise<number>, force: boolean) {
  if (!force && Date.now() - loadedAt[key] < REFRESH_MS) return
  loadedAt[key] = Date.now()
  fetcher()
    .then((n) => (badges[key] = n))
    .catch(() => {})
}

/** 用户端「我的反馈」未读数（菜单 getter 里调用，自动节流） */
export function unreadBadge(force = false): number {
  poll('unread', userApi.unread, force)
  return badges.unread
}

/** 管理端「反馈管理」待处理数 */
export function pendingBadge(force = false): number {
  poll('pending', adminApi.pendingCount, force)
  return badges.pending
}

// ─── 提交面板 ──────────────────────────────────────────────────────────

export const composer = reactive({
  open: false,
  from: '', // 打开时所在页面，写入自动采集信息
  prevId: undefined as number | undefined,
  submitted: 0 // 每次提交成功 +1，列表页据此刷新
})

export function openComposer(from: string, prevId?: number) {
  composer.from = from
  composer.prevId = prevId
  composer.open = true
}
