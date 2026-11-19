// [plus] 「用户触达」侧边栏分组；AppSidebar 只引用 outreachAdminNav
import { h } from 'vue'
import { EmailVerifyIcon } from '../emailverify/nav'
import { EMAIL_VERIFY_ADMIN_PATH } from '../emailverify/routes'
import { CAMPAIGNS_PATH, OUTREACH_ADMIN_PATH } from './routes'

const outline = (d: string) => ({
  render: () =>
    h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' }, [
      h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', d })
    ])
})

const SendIcon = outline('M6 12L3.269 3.126A59.768 59.768 0 0121.485 12 59.77 59.77 0 013.27 20.876L5.999 12zm0 0h7.5')
const TemplateIcon = outline(
  'M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z'
)
const LayoutIcon = outline(
  'M2.25 7.125C2.25 6.504 2.754 6 3.375 6h6c.621 0 1.125.504 1.125 1.125v3.75c0 .621-.504 1.125-1.125 1.125h-6a1.125 1.125 0 01-1.125-1.125v-3.75zM14.25 8.625c0-.621.504-1.125 1.125-1.125h5.25c.621 0 1.125.504 1.125 1.125v8.25c0 .621-.504 1.125-1.125 1.125h-5.25a1.125 1.125 0 01-1.125-1.125v-8.25zM3.75 16.125c0-.621.504-1.125 1.125-1.125h5.25c.621 0 1.125.504 1.125 1.125v2.25c0 .621-.504 1.125-1.125 1.125h-5.25a1.125 1.125 0 01-1.125-1.125v-2.25z'
)
const SettingsIcon = outline(
  'M10.5 6h9.75M10.5 6a1.5 1.5 0 11-3 0m3 0a1.5 1.5 0 10-3 0M3.75 6H7.5m3 12h9.75m-9.75 0a1.5 1.5 0 01-3 0m3 0a1.5 1.5 0 00-3 0m-3.75 0H7.5m9-6h3.75m-3.75 0a1.5 1.5 0 01-3 0m3 0a1.5 1.5 0 00-3 0m-9.75 0h9.75'
)

export const outreachAdminNav = {
  path: OUTREACH_ADMIN_PATH,
  label: '用户触达',
  icon: EmailVerifyIcon,
  expandOnly: true,
  children: [
    { path: CAMPAIGNS_PATH, label: '触达活动', icon: SendIcon },
    { path: `${OUTREACH_ADMIN_PATH}/templates`, label: '邮件模板', icon: TemplateIcon },
    { path: `${OUTREACH_ADMIN_PATH}/layouts`, label: '邮件外壳', icon: LayoutIcon },
    { path: EMAIL_VERIFY_ADMIN_PATH, label: '邮箱验证', icon: EmailVerifyIcon },
    { path: `${OUTREACH_ADMIN_PATH}/settings`, label: '发送设置', icon: SettingsIcon }
  ]
}
