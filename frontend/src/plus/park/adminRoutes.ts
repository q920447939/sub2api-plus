// [plus] 积分乐园管理端路由：一个模块一个页面，每页既有数据也有配置
import type { RouteRecordRaw } from 'vue-router'
import { PARK_ADMIN_BASE } from './adminNav'

const meta = (title: string) => ({ requiresAuth: true, requiresAdmin: true, title })

export const plusParkAdminRoutes: RouteRecordRaw[] = [
  { path: PARK_ADMIN_BASE, redirect: `${PARK_ADMIN_BASE}/overview` },
  {
    path: `${PARK_ADMIN_BASE}/overview`,
    name: 'PlusAdminParkOverview',
    component: () => import('./AdminOverviewView.vue'),
    meta: meta('积分乐园概览')
  },
  {
    path: `${PARK_ADMIN_BASE}/checkin`,
    name: 'PlusAdminParkCheckin',
    component: () => import('./AdminCheckinView.vue'),
    meta: meta('签到管理')
  },
  {
    path: `${PARK_ADMIN_BASE}/grid`,
    name: 'PlusAdminParkGrid',
    component: () => import('./AdminGridView.vue'),
    meta: meta('幸运九宫格')
  },
  {
    path: `${PARK_ADMIN_BASE}/points`,
    name: 'PlusAdminParkPoints',
    component: () => import('./AdminPointsView.vue'),
    meta: meta('积分管理')
  },
  { path: '/admin/points-slot', redirect: `${PARK_ADMIN_BASE}/grid` },
  { path: '/admin/lottery', redirect: `${PARK_ADMIN_BASE}/overview` }
]
