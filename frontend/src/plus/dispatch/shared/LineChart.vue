<!-- [plus] 折线 / 堆叠面积图（SVG，无依赖）：鼠标移上去看每个时间点各条线的值，点图例隐藏某条线 -->
<template>
  <div>
    <div v-if="legend" class="mb-1 flex flex-wrap gap-x-4 gap-y-1 text-xs">
      <button
        v-for="l in lines"
        :key="l.id"
        type="button"
        class="inline-flex items-center gap-1.5"
        :class="hidden[l.id] ? 'opacity-30' : ''"
        @click="hidden[l.id] = !hidden[l.id]"
      >
        <i class="inline-block w-4" :style="{ borderTop: `${l.width && l.width > 2 ? 3 : 2}px ${l.dashed ? 'dashed' : 'solid'} ${l.color}` }" />
        {{ l.name }}
      </button>
      <span v-if="lines.length > 1" class="text-gray-400">（点图例可隐藏）</span>
    </div>
    <div v-if="!hasData" class="py-8 text-center text-sm text-gray-400">没有数据</div>
    <svg v-else :viewBox="`0 0 ${W} ${H}`" class="block w-full" @mousemove="hover" @mouseleave="hi = -1">
      <g v-for="(t, k) in yTicks" :key="'y' + k">
        <line :x1="L" :x2="W - R" :y1="y(t)" :y2="y(t)" stroke="#e2e8f0" />
        <text :x="L - 6" :y="y(t) + 4" font-size="11" text-anchor="end" fill="#94a3b8">{{ fmt(t) }}</text>
      </g>
      <text v-for="(i, k) in xTicks" :key="'x' + k" :x="x(i)" :y="H - 8" font-size="11" text-anchor="middle" fill="#94a3b8">{{ labels[i] }}</text>
      <template v-if="stacked">
        <path v-for="a in areas" :key="'a' + a.id" :d="a.d" :fill="a.color" fill-opacity="0.75" stroke="#fff" stroke-width="0.5" />
      </template>
      <template v-else>
        <polyline
          v-for="l in shown"
          :key="'l' + l.id"
          :points="points(l.values)"
          fill="none"
          :stroke="l.color"
          :stroke-width="l.width ?? 1.6"
          :stroke-opacity="l.opacity ?? 1"
          :stroke-dasharray="l.dashed ? '5 4' : undefined"
        />
      </template>
      <g v-for="(m, k) in markers" :key="'m' + k">
        <line :x1="x(m.index)" :x2="x(m.index)" :y1="T" :y2="H - B" stroke="#334155" stroke-dasharray="3 3" />
        <text :x="x(m.index) + 4" :y="T + 12" font-size="11" fill="#334155">{{ m.label }}</text>
      </g>
      <line v-if="hi >= 0" :x1="x(hi)" :x2="x(hi)" :y1="T" :y2="H - B" stroke="#334155" stroke-opacity="0.4" />
    </svg>
    <div v-if="hasData" class="mt-1 min-h-[34px] rounded-lg border border-gray-100 bg-gray-50 px-3 py-1.5 text-xs leading-6 dark:border-dark-700 dark:bg-dark-800">
      <template v-if="hi >= 0">
        <b class="mr-2">{{ labels[hi] }}</b>
        <span v-for="l in shown" :key="'t' + l.id" class="mr-3 whitespace-nowrap" :style="{ color: l.color }">
          {{ l.name }}：{{ l.values[hi] == null ? '—' : fmt(l.values[hi] as number) }}
        </span>
      </template>
      <span v-else class="text-gray-400">把鼠标移到图上，看每个时间点各条线的数值</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'

export interface ChartLine {
  id: string | number
  name: string
  color: string
  values: (number | null)[]
  width?: number
  dashed?: boolean
  opacity?: number
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

const W = 900
const L = 58
const R = 14
const T = 12
const B = 28
const H = computed(() => props.height)
const hidden = reactive<Record<string, boolean>>({})
const hi = ref(-1)

const shown = computed(() => props.lines.filter((l) => !hidden[l.id]))
const n = computed(() => props.labels.length)
const all = computed(() => shown.value.flatMap((l) => l.values).filter((v): v is number => v != null && Number.isFinite(v)))
const hasData = computed(() => all.value.length > 0)

const range = computed(() => {
  if (props.stacked) return [0, 100]
  let mn = Math.min(...all.value)
  let mx = Math.max(...all.value)
  if (props.zeroBased || (mn > 0 && mn < mx * 0.35)) mn = 0
  const pad = (mx - mn) * 0.08 || 1
  return [Math.max(0, mn - pad), mx + pad]
})
const x = (i: number) => L + (n.value <= 1 ? 0 : (i / (n.value - 1)) * (W - L - R))
const y = (v: number) => {
  const [mn, mx] = range.value
  return T + (1 - (v - mn) / (mx - mn || 1)) * (H.value - T - B)
}
const yTicks = computed(() => {
  const [mn, mx] = range.value
  return [0, 1, 2, 3, 4].map((k) => mn + ((mx - mn) * k) / 4)
})
const xTicks = computed(() => {
  const step = Math.max(1, Math.ceil(n.value / 8))
  const out: number[] = []
  for (let i = 0; i < n.value; i += step) out.push(i)
  return out
})

function points(vals: (number | null)[]): string {
  return vals
    .map((v, i) => (v == null ? '' : `${x(i).toFixed(1)},${y(v).toFixed(1)}`))
    .filter(Boolean)
    .join(' ')
}

// 堆叠面积：每条线的值是份额（%），按顺序叠起来
const areas = computed(() => {
  const base = new Array(n.value).fill(0)
  return shown.value.map((l) => {
    const lower = [...base]
    const upper = l.values.map((v, i) => (base[i] += v ?? 0))
    const top = upper.map((v, i) => `${x(i).toFixed(1)},${y(Math.min(100, v)).toFixed(1)}`)
    const bottom = lower.map((v, i) => `${x(i).toFixed(1)},${y(Math.min(100, v)).toFixed(1)}`).reverse()
    return { id: l.id, color: l.color, d: `M${top.join(' L')} L${bottom.join(' L')} Z` }
  })
})

function hover(e: MouseEvent) {
  const el = e.currentTarget as SVGSVGElement
  const r = el.getBoundingClientRect()
  const xv = ((e.clientX - r.left) / r.width) * W
  const i = Math.round(((xv - L) / (W - L - R)) * (n.value - 1))
  hi.value = Math.max(0, Math.min(n.value - 1, i))
}
</script>
