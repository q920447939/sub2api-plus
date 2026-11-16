// [plus] 反馈中心的侧边栏入口；AppSidebar 只引用 feedbackUserNav / feedbackAdminNav
import { h } from 'vue'
import { FEEDBACK_ADMIN_PATH, FEEDBACK_PATH } from './routes'
import { feedbackNavFlag, pendingBadge, unreadBadge } from './state'

const outline = (d: string) => ({
  render: () =>
    h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' }, [
      h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', d })
    ])
})

const InboxIcon = outline(
  'M2.25 13.5h3.86a2.25 2.25 0 012.012 1.244l.256.512a2.25 2.25 0 002.013 1.244h3.218a2.25 2.25 0 002.013-1.244l.256-.512a2.25 2.25 0 012.013-1.244h3.859M2.25 13.5V18A2.25 2.25 0 004.5 20.25h15A2.25 2.25 0 0021.75 18v-4.5M2.25 13.5l2.394-7.183A2.25 2.25 0 016.78 4.5h10.44a2.25 2.25 0 012.136 1.817L21.75 13.5'
)
const StatsIcon = outline('M3 13.125V19.5h4.5v-6.375H3zm6.75-4.5V19.5h4.5V8.625h-4.5zm6.75-4.5V19.5H21V4.125h-4.5z')
const SettingsIcon = outline(
  'M10.5 6h9.75M10.5 6a1.5 1.5 0 11-3 0m3 0a1.5 1.5 0 10-3 0M3.75 6H7.5m3 12h9.75m-9.75 0a1.5 1.5 0 01-3 0m3 0a1.5 1.5 0 00-3 0m-3.75 0H7.5m9-6h3.75m-3.75 0a1.5 1.5 0 01-3 0m3 0a1.5 1.5 0 00-3 0m-9.75 0h9.75'
)

export const FeedbackNavIcon = {
  render: () =>
    h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' }, [
      h('path', {
        'stroke-linecap': 'round',
        'stroke-linejoin': 'round',
        d: 'M7.5 8.25h9m-9 3H12m-9.75 1.51c0 1.6 1.123 2.994 2.707 3.227 1.129.166 2.27.293 3.423.379.35.026.67.21.865.501L12 21l2.755-4.133a1.14 1.14 0 01.865-.501 48.172 48.172 0 003.423-.379c1.584-.233 2.707-1.626 2.707-3.228V6.741c0-1.602-1.123-2.995-2.707-3.228A48.394 48.394 0 0012 3c-2.392 0-4.744.175-7.043.513C3.373 3.746 2.25 5.14 2.25 6.741v6.018z'
      })
    ])
}

export const feedbackUserNav = {
  path: FEEDBACK_PATH,
  label: '我的反馈',
  icon: FeedbackNavIcon,
  featureFlag: feedbackNavFlag,
  badge: () => unreadBadge()
}

// 管理端分组（与智商检测、调度台一致）：管理端入口不跟随用户端开关，关掉反馈后历史数据仍要能看。
// 分组收起时红点挂在父级，展开后挂在「反馈处理」上。
const pending = () => pendingBadge()

export const feedbackAdminNav = {
  path: FEEDBACK_ADMIN_PATH,
  label: '反馈管理',
  icon: FeedbackNavIcon,
  expandOnly: true,
  badge: pending,
  children: [
    { path: FEEDBACK_ADMIN_PATH, label: '反馈处理', icon: InboxIcon, badge: pending },
    { path: `${FEEDBACK_ADMIN_PATH}/stats`, label: '反馈看板', icon: StatsIcon },
    { path: `${FEEDBACK_ADMIN_PATH}/settings`, label: '反馈设置', icon: SettingsIcon }
  ]
}
