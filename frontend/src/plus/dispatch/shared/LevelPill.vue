<!-- [plus] 等级小标签：正常 / 可疑 / 异常 / 数据不足 / 未检测 / 只展示 -->
<template>
  <span class="inline-flex items-center gap-1.5 whitespace-nowrap rounded-md border px-2 py-0.5 text-xs" :class="LEVEL_CLASS[level]">
    <i v-if="level !== 'info'" class="inline-block h-1.5 w-1.5 rounded-full" :style="{ background: level === 'none' ? 'transparent' : 'currentColor', border: level === 'none' ? '1px solid #cbd5e1' : '' }" />
    {{ text ?? LEVEL_TEXT[level] }}
  </span>
</template>

<script setup lang="ts">
import type { Level } from '../api'
import { LEVEL_CLASS, LEVEL_TEXT } from './format'

defineProps<{ level: Level; text?: string }>()
</script>
