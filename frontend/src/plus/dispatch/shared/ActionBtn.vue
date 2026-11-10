<!-- [plus] 表格行操作按钮（和上游列表页一致：图标 + 小字） -->
<template>
  <component
    :is="to ? 'router-link' : 'button'"
    :to="to"
    :type="to ? undefined : 'button'"
    :disabled="disabled"
    class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors disabled:cursor-not-allowed disabled:opacity-40 dark:text-dark-300"
    :class="danger ? 'hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400' : 'hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400'"
  >
    <Icon :name="icon" size="sm" />
    <span class="whitespace-nowrap text-xs">{{ label }}</span>
  </component>
</template>

<script setup lang="ts">
import type { RouteLocationRaw } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'

defineProps<{ icon: any; label: string; danger?: boolean; disabled?: boolean; to?: RouteLocationRaw }>()
</script>
