<!-- [plus] 权重按钮：正常 / 暂停 / 40% / 已调 N 个分组，点开调整 -->
<template>
  <button
    type="button" class="whitespace-nowrap rounded-lg px-3 py-1 text-sm"
    :class="f.tone === 'off' ? 'bg-gray-900 font-semibold text-white dark:bg-white dark:text-gray-900' : f.tone === 'part' ? 'bg-primary-50 font-semibold text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-dark-200'"
    data-testid="dispatch-flow" @click="emit('click')"
  >{{ f.text }}</button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Rule } from '../api'
import { flowText } from './format'

const props = defineProps<{ rules: Rule[]; groups: number; model?: string }>()
const emit = defineEmits<{ click: [] }>()
const f = computed(() => flowText(props.rules, props.groups, props.model ?? null))
</script>
