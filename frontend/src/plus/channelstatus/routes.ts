// [plus] 渠道状态路由（用户端 + 管理端），router/index.ts 只展开这一个数组
import type { RouteRecordRaw } from 'vue-router'

export const CHANNEL_STATUS_PATH = '/channel-status'
export const CHANNEL_STATUS_ADMIN_PATH = '/admin/channel-status'

export const plusChannelStatusRoutes: RouteRecordRaw[] = [
  {
    path: CHANNEL_STATUS_PATH,
    name: 'PlusChannelStatus',
    component: () => import('./ChannelStatusView.vue'),
    meta: { requiresAuth: true, requiresAdmin: false, title: '渠道状态' }
  },
  {
    path: CHANNEL_STATUS_ADMIN_PATH,
    name: 'PlusAdminChannelStatus',
    component: () => import('./admin/ChannelStatusSettingsView.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, title: '渠道状态设置' }
  }
]
