<!-- [plus] 一次检测的摘要：结果、时间、耗时、token，最像的模型 / 回答和标准答案 / 作品缩略图 -->
<template>
  <div class="rounded-xl bg-gray-50 p-3 text-sm dark:bg-dark-800">
    <div class="flex flex-wrap items-center gap-2">
      <span class="font-semibold" :class="run.outcome === 'pass' || run.outcome === 'saved' ? 'text-emerald-600' : run.outcome === 'fail' ? 'text-red-600' : 'text-gray-500'">{{ outcomeOf(spec, run.outcome).label }}</span>
      <span class="text-xs text-gray-500">{{ meta }}</span>
      <button type="button" class="ml-auto text-xs text-primary-600 hover:underline" @click="emit('open', run.id)">{{ spec.gallery ? '看作品' : '看完整回答' }}</button>
    </div>
    <div v-if="run.summary" class="mt-1">{{ run.summary }}</div>
    <div v-if="run.preview_url" class="mt-2 w-48 cursor-pointer overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700" @click="emit('open', run.id)">
      <PreviewFrame :src="run.preview_url" thumb />
    </div>
    <template v-if="top.length">
      <div class="mt-2 text-xs text-gray-400">最像的模型</div>
      <div v-for="x in top" :key="x.model" class="grid grid-cols-[minmax(0,1.2fr)_1fr_44px] items-center gap-2 py-0.5">
        <span class="truncate font-mono text-xs">{{ x.model }}</span>
        <span class="h-1.5 overflow-hidden rounded bg-gray-200 dark:bg-dark-600"><i class="block h-full rounded bg-gray-500" :style="{ width: `${Math.max(0, Math.min(100, x.probability))}%` }" /></span>
        <b class="text-right text-xs tabular-nums">{{ Math.round(x.probability) }}%</b>
      </div>
    </template>
    <div v-if="run.detail?.answer != null" class="mt-2 flex gap-3 text-xs">
      <span class="text-gray-500">回答</span><b>{{ run.detail.answer || '没答出' }}</b>
      <span class="ml-auto text-gray-400">标准答案 {{ (run.detail.expected ?? []).join(' / ') }}</span>
    </div>
    <div v-if="run.error" class="mt-2 break-all text-xs text-red-600">{{ run.error }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Run, TypeSpec } from '../api'
import PreviewFrame from '@/plus/iq/shared/PreviewFrame.vue'
import { dt, num, outcomeOf, TRIGGER } from './format'

const props = defineProps<{ run: Run; spec: TypeSpec }>()
const emit = defineEmits<{ open: [id: number] }>()

const top = computed(() => (Array.isArray(props.run.detail?.top) ? (props.run.detail.top as { model: string; probability: number }[]).slice(0, 5) : []))
const meta = computed(() => {
  const r = props.run
  return [
    dt(r.queued_at),
    TRIGGER[r.trigger] ?? r.trigger,
    r.duration_ms != null ? `${(r.duration_ms / 1000).toFixed(1)} 秒` : '',
    r.input_tokens != null ? `输入 ${num(r.input_tokens)} · 输出 ${num(r.output_tokens ?? 0)} token` : ''
  ]
    .filter(Boolean)
    .join(' · ')
})
</script>
