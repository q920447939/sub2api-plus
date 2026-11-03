<template>
  <div class="card p-5">
    <p class="mb-3 font-medium text-gray-900 dark:text-white">{{ title }}</p>
    <div class="h-64">
      <Line :data="chartData" :options="options" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Line } from 'vue-chartjs'
import { CategoryScale, Chart as ChartJS, Legend, LinearScale, LineElement, PointElement, Tooltip } from 'chart.js'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend)

const props = defineProps<{
  title: string
  labels: string[]
  series: { label: string; data: number[]; color: string }[]
}>()

const chartData = computed(() => ({
  labels: props.labels.map((d) => d.slice(5)), // MM-DD
  datasets: props.series.map((s) => ({
    label: s.label,
    data: s.data,
    borderColor: s.color,
    backgroundColor: s.color,
    tension: 0.3,
    pointRadius: props.labels.length > 14 ? 0 : 3
  }))
}))

const options = {
  responsive: true,
  maintainAspectRatio: false,
  interaction: { mode: 'index' as const, intersect: false },
  plugins: { legend: { position: 'bottom' as const } },
  scales: { y: { beginAtZero: true, ticks: { precision: 0 } } }
}
</script>
