// [plus] 邮箱验证路由（管理端 /admin/email-verify）
import type { RouteRecordRaw } from 'vue-router'

export const EMAIL_VERIFY_ADMIN_PATH = '/admin/email-verify'

export const plusEmailVerifyRoutes: RouteRecordRaw[] = [
  {
    path: '/verify-email',
    name: 'PlusVerifyEmail',
    component: () => import('./VerifyEmailView.vue'),
    meta: { requiresAuth: false, title: '验证邮箱' }
  },
  {
    path: EMAIL_VERIFY_ADMIN_PATH,
    name: 'PlusAdminEmailVerify',
    component: () => import('./admin/EmailVerifyOverviewView.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, title: '邮箱验证' }
  }
]
