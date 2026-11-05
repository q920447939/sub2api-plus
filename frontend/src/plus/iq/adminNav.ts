// [plus] 管理端侧边栏「智商检测」分组；AppSidebar 只引用这一项。
import { IQ_ADMIN_BASE } from './routes'
import { GalleryIcon, IQAdminIcon, RunsIcon, SettingsIcon, TasksIcon } from './navIcon'

export const iqAdminNav = {
  path: IQ_ADMIN_BASE,
  label: '智商检测',
  icon: IQAdminIcon,
  expandOnly: true,
  children: [
    { path: `${IQ_ADMIN_BASE}/gallery`, label: '检测画廊', icon: GalleryIcon },
    { path: `${IQ_ADMIN_BASE}/tasks`, label: '检测任务', icon: TasksIcon },
    { path: `${IQ_ADMIN_BASE}/runs`, label: '运行记录', icon: RunsIcon },
    { path: `${IQ_ADMIN_BASE}/settings`, label: '设置', icon: SettingsIcon }
  ]
}
