// [plus] 上游调度台路由（只有管理端）。router/index.ts 只展开 plusDispatchRoutes 这一个数组。
import type { RouteRecordRaw } from 'vue-router'

export const DISPATCH_BASE = '/admin/dispatch'

const admin = (path: string, name: string, title: string, component: () => Promise<unknown>): RouteRecordRaw => ({
  path: `${DISPATCH_BASE}/${path}`,
  name,
  component: component as RouteRecordRaw['component'],
  meta: { requiresAuth: true, requiresAdmin: true, title }
} as RouteRecordRaw)

export const plusDispatchRoutes: RouteRecordRaw[] = [
  { path: DISPATCH_BASE, redirect: `${DISPATCH_BASE}/board` },
  admin('board', 'PlusDispatchBoard', '上游账号', () => import('./views/BoardView.vue')),
  admin('account/:id', 'PlusDispatchAccount', '账号详情', () => import('./views/AccountView.vue')),
  admin('changes', 'PlusDispatchChanges', '调整记录', () => import('./views/ChangesView.vue')),
  admin('runs', 'PlusDispatchRuns', '检测记录', () => import('./views/RunsView.vue')),
  admin('config', 'PlusDispatchConfig', '检测配置', () => import('./views/ConfigView.vue')),
  admin('audit', 'PlusDispatchAudit', '操作日志', () => import('./views/AuditView.vue'))
]
