// [plus] 管理端侧边栏「上游调度台」分组；AppSidebar 只引用这一项。
import { DISPATCH_BASE } from './routes'
import { AdjustIcon, AuditIcon, CompareIcon, ConfigIcon, DispatchIcon, EventsIcon, OverviewIcon, RunsIcon } from './navIcon'

export const dispatchAdminNav = {
  path: DISPATCH_BASE,
  label: '上游调度台',
  icon: DispatchIcon,
  expandOnly: true,
  children: [
    { path: `${DISPATCH_BASE}/overview`, label: '健康总览', icon: OverviewIcon },
    { path: `${DISPATCH_BASE}/compare`, label: '指标对比', icon: CompareIcon },
    { path: `${DISPATCH_BASE}/runs`, label: '检测记录', icon: RunsIcon },
    { path: `${DISPATCH_BASE}/events`, label: '异常待处理', icon: EventsIcon },
    { path: `${DISPATCH_BASE}/adjust`, label: '调度调整', icon: AdjustIcon },
    { path: `${DISPATCH_BASE}/config`, label: '检测配置', icon: ConfigIcon },
    { path: `${DISPATCH_BASE}/audit`, label: '操作日志', icon: AuditIcon }
  ]
}
