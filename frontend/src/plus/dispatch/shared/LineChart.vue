<!-- [plus] 折线 / 堆叠面积图：用项目统一的 chart.js（vue-chartjs），悬停显示同一时刻各条线的值，点图例显示 / 隐藏 -->
<template>
  <div :style="{ height: `${height}px` }">
    <div v-if="!hasData" class="flex h-full items-center justify-center text-sm text-gray-400">暂无数据</div>
    <Line v-else :data="chartData" :options="options" :plugins="[markerPlugin]" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Chart as ChartJS, CategoryScale, Filler, Legend, LinearScale, LineElement, PointElement, Tooltip, type Plugin } from 'chart.js'
import { Line } from 'vue-chartjs'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Filler)

export interface ChartLine {
  id: string | number
  name: string
  color: string
  values: (number | null)[]
  width?: number
  dashed?: boolean
  opacity?: number
  /** 默认隐藏，点图例再显示 */
  hidden?: boolean
}

const props = withDefaults(
  defineProps<{
    lines: ChartLine[]
    labels: string[]
    fmt?: (v: number) => string
    stacked?: boolean
    legend?: boolean
    markers?: { index: number; label: string }[]
    height?: number
    zeroBased?: boolean
  }>(),
  { fmt: (v: number) => String(Math.round(v * 10) / 10), stacked: false, legend: true, markers: () => [], height: 260, zeroBased: false }
)

const dark = computed(() => document.documentElement.classList.contains('dark'))
const text = computed(() => (dark.value ? '#9ca3af' : '#6b7280'))
const grid = computed(() => (dark.value ? '#374151' : '#f1f5f9'))

const hasData = computed(() => props.lines.some((l) => l.values.some((v) => v != null && Number.isFinite(v))))

const alpha = (hex: string, a: number) => {
  const n = Math.round(Math.max(0, Math.min(1, a)) * 255).toString(16).padStart(2, '0')
  return /^#[0-9a-f]{6}$/i.test(hex) ? hex + n : hex
}

const chartData = computed(() => ({
  labels: props.labels,
  datasets: props.lines.map((l) => ({
    label: l.name,
    data: props.stacked ? l.values.map((v) => v ?? 0) : l.values,
    hidden: !!l.hidden,
    borderColor: alpha(l.color, l.opacity ?? 1),
    backgroundColor: props.stacked ? alpha(l.color, 0.55) : l.color,
    borderWidth: props.stacked ? 1 : l.width ?? 1.8,
    borderDash: l.dashed ? [5, 4] : undefined,
    fill: props.stacked ? (props.lines.indexOf(l) === 0 ? 'origin' : '-1') : false,
    tension: props.stacked ? 0 : 0.3,
    spanGaps: true,
    // 点很少时（新账号、检测频率低）画出数据点，否则一两个点看不见
    pointRadius: !props.stacked && l.values.filter((v) => v != null).length <= 12 ? 2.5 : 0,
    pointHoverRadius: l.dashed ? 0 : 4
  }))
}))

const options = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: false as const,
  interaction: { intersect: false, mode: 'index' as const },
  plugins: {
    legend: {
      display: props.legend,
      position: 'top' as const,
      align: 'start' as const,
      labels: { color: text.value, usePointStyle: true, pointStyle: 'line', boxWidth: 18, padding: 14, font: { size: 11 } }
    },
    tooltip: {
      itemSort: (a: any, b: any) => (b.raw ?? -Infinity) - (a.raw ?? -Infinity),
      callbacks: {
        label: (c: any) => (c.raw == null ? `${c.dataset.label}：—` : `${c.dataset.label}：${props.fmt(Number(c.raw))}`)
      }
    }
  },
  scales: {
    x: { grid: { display: false }, ticks: { color: text.value, font: { size: 10 }, maxRotation: 0, autoSkipPadding: 16 } },
    y: {
      stacked: props.stacked,
      beginAtZero: props.zeroBased || props.stacked,
      max: props.stacked ? 100 : undefined,
      grid: { color: grid.value },
      border: { display: false },
      ticks: { color: text.value, font: { size: 10 }, maxTicksLimit: 5, callback: (v: string | number) => props.fmt(Number(v)) }
    }
  }
}))

// 竖线标记（比如「调整」时刻）
const markerPlugin: Plugin<'line'> = {
  id: 'plusMarkers',
  afterDatasetsDraw(chart) {
    const { ctx, chartArea, scales } = chart
    for (const m of props.markers) {
      const x = scales.x.getPixelForValue(m.index)
      ctx.save()
      ctx.strokeStyle = text.value
      ctx.setLineDash([3, 3])
      ctx.beginPath()
      ctx.moveTo(x, chartArea.top)
      ctx.lineTo(x, chartArea.bottom)
      ctx.stroke()
      ctx.setLineDash([])
      ctx.fillStyle = text.value
      ctx.font = '11px sans-serif'
      ctx.fillText(m.label, x + 4, chartArea.top + 11)
      ctx.restore()
    }
  }
}
</script>
