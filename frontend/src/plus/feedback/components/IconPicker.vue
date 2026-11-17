<!-- [plus] 场景图标选择：常用 emoji 一键选择，也可以输入任意 emoji -->
<template>
  <div ref="root" class="relative" @keydown.esc="onEsc">
    <button
      ref="trigger"
      type="button"
      class="flex h-[42px] w-[42px] items-center justify-center rounded-xl border border-gray-200 bg-white text-xl transition-colors hover:border-primary-400 dark:border-dark-600 dark:bg-dark-800"
      :class="open ? 'border-primary-500 ring-2 ring-primary-500/20' : ''"
      aria-label="选择图标"
      data-testid="icon-picker"
      @click="open = !open"
    >
      {{ modelValue || '💬' }}
    </button>
    <div v-if="open" class="absolute left-0 top-full z-20 mt-2 w-72 rounded-xl border border-gray-200 bg-white p-3 shadow-lg dark:border-dark-600 dark:bg-dark-800">
      <div class="grid grid-cols-8 gap-1">
        <button
          v-for="e in PRESETS"
          :key="e"
          type="button"
          class="flex h-8 w-8 items-center justify-center rounded-lg text-lg transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
          :class="e === modelValue ? 'bg-primary-50 ring-1 ring-primary-400 dark:bg-primary-900/30' : ''"
          @click="pick(e)"
        >
          {{ e }}
        </button>
      </div>
      <input
        v-model.trim="custom"
        maxlength="4"
        placeholder="或粘贴其他 emoji"
        class="input mt-3 h-9 text-sm"
        @keydown.enter.prevent="custom && pick(custom)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { onClickOutside } from '@vueuse/core'

defineProps<{ modelValue: string }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

const PRESETS = [
  '💬', '⚠️', '🐞', '🔌', '🧠', '🐢', '💳', '🎁',
  '👤', '🔑', '📊', '🖼️', '📱', '🌐', '⏱️', '🔒',
  '✨', '🧩', '🎯', '🏷️', '🎉', '💡', '📚', '🚀',
  '❓', '📝', '⚙️', '🔔', '📦', '🤖', '🛠️', '❤️'
]

const open = ref(false)
const custom = ref('')
const root = ref<HTMLElement>()
const trigger = ref<HTMLButtonElement>()
onClickOutside(root, () => (open.value = false))

// Esc 只收起弹层，不让外层对话框跟着关闭
function onEsc(e: KeyboardEvent) {
  if (!open.value) return
  e.stopPropagation()
  open.value = false
  trigger.value?.focus()
}

function pick(e: string) {
  emit('update:modelValue', e)
  custom.value = ''
  open.value = false
}
</script>
