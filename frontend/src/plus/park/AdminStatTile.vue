<template>
  <div class="card p-4">
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ label }}</p>
    <p class="mt-1 text-2xl font-semibold tabular-nums text-gray-900 dark:text-white">{{ value }}</p>
    <p v-if="sub" class="mt-1 text-xs text-gray-400" :class="subClass">{{ sub }}</p>
    <slot />
  </div>
</template>

<script setup lang="ts">
defineProps<{ label: string; value: string | number; sub?: string; subClass?: string }>()
</script>
