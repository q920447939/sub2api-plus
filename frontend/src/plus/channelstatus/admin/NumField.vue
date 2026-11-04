<template>
  <label class="block text-sm text-gray-600 dark:text-dark-300">
    {{ label }}
    <div class="relative mt-1">
      <input
        :value="modelValue"
        type="number"
        class="input pr-14"
        :min="min"
        :max="max"
        :step="step ?? 1"
        @input="onInput"
      />
      <span v-if="suffix" class="pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-gray-400">{{ suffix }}</span>
    </div>
  </label>
</template>

<script setup lang="ts">
defineProps<{ modelValue: number; label: string; suffix?: string; min?: number; max?: number; step?: number }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: number): void }>()

function onInput(e: Event) {
  const v = Number((e.target as HTMLInputElement).value)
  if (Number.isFinite(v)) emit('update:modelValue', v)
}
</script>
