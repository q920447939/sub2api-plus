// [plus] 用户触达路由：管理端 /admin/outreach/*，用户端 /outreach/claim，免登录 /unsubscribe
import type { RouteRecordRaw } from 'vue-router'

export const OUTREACH_ADMIN_PATH = '/admin/outreach'
/** 活动单独一个前缀：侧边栏按前缀高亮，活动放在根路径会让「模板 / 外壳 / 发送设置」页也高亮「触达活动」 */
export const CAMPAIGNS_PATH = `${OUTREACH_ADMIN_PATH}/campaigns`

const adminMeta = (title: string) => ({ requiresAuth: true, requiresAdmin: true, title })
const P = OUTREACH_ADMIN_PATH
const C = CAMPAIGNS_PATH

export const plusOutreachRoutes: RouteRecordRaw[] = [
  { path: P, redirect: C },
  { path: C, name: 'PlusAdminOutreach', component: () => import('./admin/CampaignListView.vue'), meta: adminMeta('触达活动') },
  { path: `${C}/new`, name: 'PlusAdminOutreachNew', component: () => import('./admin/CampaignEditView.vue'), meta: adminMeta('新建活动') },
  { path: `${C}/:id(\\d+)/edit`, name: 'PlusAdminOutreachEdit', component: () => import('./admin/CampaignEditView.vue'), meta: adminMeta('编辑活动') },
  { path: `${C}/:id(\\d+)`, name: 'PlusAdminOutreachDetail', component: () => import('./admin/CampaignDetailView.vue'), meta: adminMeta('活动详情') },
  { path: `${C}/batches/:id(\\d+)`, name: 'PlusAdminOutreachBatch', component: () => import('./admin/BatchDetailView.vue'), meta: adminMeta('批次详情') },
  { path: `${P}/templates`, name: 'PlusAdminOutreachTemplates', component: () => import('./admin/TemplateListView.vue'), meta: adminMeta('邮件模板') },
  { path: `${P}/templates/new`, name: 'PlusAdminOutreachTemplateNew', component: () => import('./admin/TemplateEditView.vue'), meta: adminMeta('新建模板') },
  { path: `${P}/templates/:id(\\d+)`, name: 'PlusAdminOutreachTemplateEdit', component: () => import('./admin/TemplateEditView.vue'), meta: adminMeta('编辑模板') },
  { path: `${P}/layouts`, name: 'PlusAdminOutreachLayouts', component: () => import('./admin/LayoutListView.vue'), meta: adminMeta('邮件外壳') },
  { path: `${P}/layouts/new`, name: 'PlusAdminOutreachLayoutNew', component: () => import('./admin/LayoutEditView.vue'), meta: adminMeta('新建外壳') },
  { path: `${P}/layouts/:id(\\d+)`, name: 'PlusAdminOutreachLayoutEdit', component: () => import('./admin/LayoutEditView.vue'), meta: adminMeta('编辑外壳') },
  { path: `${P}/settings`, name: 'PlusAdminOutreachSettings', component: () => import('./admin/SettingsView.vue'), meta: adminMeta('发送设置') },
  {
    path: '/outreach/claim',
    name: 'PlusOutreachClaim',
    component: () => import('./ClaimView.vue'),
    meta: { requiresAuth: true, requiresAdmin: false, title: '领取权益' }
  },
  {
    path: '/unsubscribe',
    name: 'PlusUnsubscribe',
    component: () => import('./UnsubscribeView.vue'),
    meta: { requiresAuth: false, title: '退订邮件' }
  }
]
