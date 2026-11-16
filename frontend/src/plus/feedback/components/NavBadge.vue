<!-- [plus] 侧边栏红点：展开时显示数字，收起时只显示圆点 -->
<template>
  <span v-if="count > 0 && collapsed" class="-ml-2.5 -mt-4 h-2 w-2 shrink-0 rounded-full bg-red-500 ring-2 ring-white dark:ring-dark-900" />
  <span v-else-if="count > 0" class="ml-auto min-w-[18px] shrink-0 rounded-full bg-red-500 px-1.5 text-center text-[11px] font-semibold leading-[18px] text-white tabular-nums">
    {{ count > 99 ? '99+' : count }}
  </span>
</template>

<script setup lang="ts">
defineProps<{ count: number; collapsed?: boolean }>()
</script>
