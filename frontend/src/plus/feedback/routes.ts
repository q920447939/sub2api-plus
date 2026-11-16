// [plus] 反馈中心路由（用户端 /feedback + 管理端 /admin/feedback），router/index.ts 只展开这一个数组
import type { RouteRecordRaw } from 'vue-router'

export const FEEDBACK_PATH = '/feedback'
export const FEEDBACK_ADMIN_PATH = '/admin/feedback'

export const plusFeedbackRoutes: RouteRecordRaw[] = [
  {
    path: FEEDBACK_PATH,
    name: 'PlusFeedback',
    component: () => import('./user/MyFeedbackView.vue'),
    meta: { requiresAuth: true, requiresAdmin: false, title: '我的反馈' }
  },
  {
    path: `${FEEDBACK_PATH}/:id(\\d+)`,
    name: 'PlusFeedbackDetail',
    component: () => import('./user/FeedbackDetailView.vue'),
    meta: { requiresAuth: true, requiresAdmin: false, title: '反馈详情' }
  },
  {
    path: FEEDBACK_ADMIN_PATH,
    name: 'PlusAdminFeedback',
    component: () => import('./admin/InboxView.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, title: '反馈管理' }
  },
  {
    path: `${FEEDBACK_ADMIN_PATH}/stats`,
    name: 'PlusAdminFeedbackStats',
    component: () => import('./admin/StatsView.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, title: '反馈看板' }
  },
  {
    path: `${FEEDBACK_ADMIN_PATH}/settings`,
    name: 'PlusAdminFeedbackSettings',
    component: () => import('./admin/SettingsView.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, title: '反馈设置' }
  },
  {
    path: `${FEEDBACK_ADMIN_PATH}/:id(\\d+)`,
    name: 'PlusAdminFeedbackDetail',
    component: () => import('./admin/DetailView.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, title: '反馈详情' }
  }
]
