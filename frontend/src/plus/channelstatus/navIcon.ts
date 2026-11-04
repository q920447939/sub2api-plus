// [plus] 渠道状态图标：心电脉冲线，与上游渠道监控的信号塔图标区分
import { h } from 'vue'

export const ChannelStatusNavIcon = {
  render: () =>
    h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' }, [
      h('path', {
        'stroke-linecap': 'round',
        'stroke-linejoin': 'round',
        d: 'M2.25 12h4.5l2.25-6 4.5 12 2.25-6h6'
      })
    ])
}
