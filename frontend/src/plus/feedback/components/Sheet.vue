<!-- [plus] 面板：移动端从底部滑出，桌面端居中卡片 -->
<template>
  <Teleport to="body">
    <Transition name="fb-sheet">
      <div v-if="open" class="fixed inset-0 z-[60] flex items-end justify-center sm:items-center sm:p-6" @keydown.esc="emit('close')">
        <div class="absolute inset-0 bg-black/30 backdrop-blur-[2px]" @click="emit('close')" />
        <section
          role="dialog"
          aria-modal="true"
          class="fb-sheet-panel relative flex max-h-[92vh] w-full flex-col overflow-hidden rounded-t-3xl bg-gray-50 shadow-2xl dark:bg-dark-900 sm:rounded-3xl"
          :class="width"
        >
          <header class="flex items-center gap-3 px-5 pb-2 pt-4">
            <h2 class="flex-1 text-base font-semibold text-gray-900 dark:text-white">{{ title }}</h2>
            <button type="button" class="flex h-8 w-8 items-center justify-center rounded-full bg-gray-200/70 text-gray-500 hover:bg-gray-200 dark:bg-dark-700 dark:text-dark-300" aria-label="关闭" @click="emit('close')">
              <svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2.2" viewBox="0 0 24 24"><path stroke-linecap="round" d="M6 6l12 12M18 6L6 18" /></svg>
            </button>
          </header>
          <div class="flex-1 overflow-y-auto px-5 pb-4">
            <slot />
          </div>
          <footer v-if="$slots.footer" class="border-t border-gray-200/70 bg-white/70 px-5 py-3 backdrop-blur dark:border-dark-700 dark:bg-dark-800/70">
            <slot name="footer" />
          </footer>
        </section>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ open: boolean; title: string; width?: string }>(), { width: 'sm:max-w-xl' })
const emit = defineEmits<{ (e: 'close'): void }>()
</script>

<style scoped>
.fb-sheet-enter-active,
.fb-sheet-leave-active {
  transition: opacity 0.2s ease;
}
.fb-sheet-enter-active .fb-sheet-panel,
.fb-sheet-leave-active .fb-sheet-panel {
  transition: transform 0.28s cubic-bezier(0.32, 0.72, 0, 1);
}
.fb-sheet-enter-from,
.fb-sheet-leave-to {
  opacity: 0;
}
.fb-sheet-enter-from .fb-sheet-panel,
.fb-sheet-leave-to .fb-sheet-panel {
  transform: translateY(24px);
}
</style>
