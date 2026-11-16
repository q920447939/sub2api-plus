<!-- [plus] 分段控件（iOS segmented control） -->
<template>
  <div class="inline-flex rounded-xl bg-gray-100 p-0.5 dark:bg-dark-800" role="tablist">
    <button
      v-for="o in options"
      :key="String(o.value)"
      type="button"
      role="tab"
      :aria-selected="o.value === modelValue"
      class="relative flex items-center gap-1.5 rounded-[10px] px-3 py-1.5 text-sm transition-all"
      :class="
        o.value === modelValue
          ? 'bg-white font-semibold text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white'
          : 'text-gray-500 hover:text-gray-800 dark:text-dark-300 dark:hover:text-white'
      "
      @click="emit('update:modelValue', o.value)"
    >
      {{ o.label }}
      <span v-if="o.count" class="rounded-full bg-gray-200/80 px-1.5 text-[11px] leading-4 tabular-nums text-gray-600 dark:bg-dark-500 dark:text-dark-200">
        {{ o.count }}
      </span>
    </button>
  </div>
</template>

<script setup lang="ts" generic="T extends string | number">
defineProps<{ modelValue: T; options: { value: T; label: string; count?: number }[] }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: T): void }>()
</script>
