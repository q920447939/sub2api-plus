<!-- [plus] 「更多」菜单：次要操作收起来，主操作留在外面 -->
<template>
  <div ref="root" class="relative">
    <button type="button" class="btn btn-secondary" :disabled="disabled" @click="open = !open">更多 ▾</button>
    <div v-if="open" class="absolute right-0 z-20 mt-1 w-40 overflow-hidden rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-dark-700 dark:bg-dark-800">
      <button
        v-for="it in items"
        :key="it.key"
        type="button"
        class="block w-full px-4 py-2 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-700"
        :class="it.danger ? 'text-red-600' : 'text-gray-700 dark:text-dark-200'"
        @click="pick(it.key)"
      >{{ it.label }}</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

defineProps<{ items: { key: string; label: string; danger?: boolean }[]; disabled?: boolean }>()
const emit = defineEmits<{ select: [key: string] }>()
const open = ref(false)
const root = ref<HTMLElement | null>(null)

function pick(key: string) {
  open.value = false
  emit('select', key)
}
function onDoc(e: MouseEvent) {
  if (root.value && !root.value.contains(e.target as Node)) open.value = false
}
onMounted(() => document.addEventListener('click', onDoc))
onBeforeUnmount(() => document.removeEventListener('click', onDoc))
</script>
