// [plus] 智商检测路由：用户端 /model-gallery 与管理端 /admin/model-iq/* 是两组独立页面（菜单即身份）。
// router/index.ts 只展开 plusIQRoutes 这一个数组。
import type { RouteRecordRaw } from 'vue-router'

export const IQ_USER_PATH = '/model-gallery'
export const IQ_ADMIN_BASE = '/admin/model-iq'

export const plusIQRoutes: RouteRecordRaw[] = [
  {
    path: IQ_USER_PATH,
    name: 'PlusModelGallery',
    component: () => import('./user/ModelGalleryView.vue'),
    meta: { requiresAuth: true, requiresAdmin: false, title: '模型画廊' }
  },
  { path: IQ_ADMIN_BASE, redirect: `${IQ_ADMIN_BASE}/gallery` },
  {
    path: `${IQ_ADMIN_BASE}/gallery`,
    name: 'PlusAdminIQGallery',
    component: () => import('./admin/GalleryView.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, title: '检测画廊' }
  },
  {
    path: `${IQ_ADMIN_BASE}/tasks`,
    name: 'PlusAdminIQTasks',
    component: () => import('./admin/TasksView.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, title: '检测任务' }
  },
  {
    path: `${IQ_ADMIN_BASE}/runs`,
    name: 'PlusAdminIQRuns',
    component: () => import('./admin/RunsView.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, title: '运行记录' }
  },
  {
    path: `${IQ_ADMIN_BASE}/settings`,
    name: 'PlusAdminIQSettings',
    component: () => import('./admin/SettingsView.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, title: '智商检测设置' }
  }
]
