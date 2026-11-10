<!-- [plus] 页内标签（下划线样式，各页统一） -->
<template>
  <nav class="flex gap-6 border-b border-gray-200 dark:border-dark-700">
    <button v-for="t in tabs" :key="t.k" type="button" class="-mb-px border-b-2 pb-2.5 text-sm transition-colors" :class="modelValue === t.k ? 'border-primary-600 font-medium text-gray-900 dark:text-white' : 'border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-dark-200'" @click="emit('update:modelValue', t.k)">
      {{ t.n }}<span v-if="t.count != null" class="ml-1.5 rounded-full bg-gray-100 px-1.5 text-xs text-gray-500 dark:bg-dark-700">{{ t.count }}</span>
    </button>
  </nav>
</template>

<script setup lang="ts">
defineProps<{ modelValue: string; tabs: { k: string; n: string; count?: number }[] }>()
const emit = defineEmits<{ 'update:modelValue': [v: string] }>()
</script>
