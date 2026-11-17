<!-- [plus] 面板：移动端从底部滑出，桌面端居中卡片。滚动锁定、Esc、焦点归还与 BaseDialog 一致 -->
<template>
  <Teleport to="body">
    <Transition name="fb-sheet">
      <div v-if="open" class="fixed inset-0 z-[60] flex items-end justify-center sm:items-center sm:p-6">
        <div class="absolute inset-0 bg-black/30 backdrop-blur-[2px]" @click="emit('close')" />
        <section
          ref="panel"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="titleId"
          tabindex="-1"
          class="fb-sheet-panel relative flex w-full flex-col overflow-hidden rounded-t-3xl bg-gray-50 shadow-2xl outline-none dark:bg-dark-900 sm:rounded-3xl"
          :class="width"
        >
          <header class="flex items-center gap-3 px-5 pb-2 pt-4">
            <h2 :id="titleId" class="flex-1 text-base font-semibold text-gray-900 dark:text-white">{{ title }}</h2>
            <button type="button" class="flex h-8 w-8 items-center justify-center rounded-full bg-gray-200/70 text-gray-500 hover:bg-gray-200 dark:bg-dark-700 dark:text-dark-300" aria-label="关闭" @click="emit('close')">
              <svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2.2" viewBox="0 0 24 24"><path stroke-linecap="round" d="M6 6l12 12M18 6L6 18" /></svg>
            </button>
          </header>
          <div class="flex-1 overflow-y-auto overscroll-contain px-5 pb-4">
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

<script lang="ts">
let sheetIdCounter = 0
</script>

<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'

const props = withDefaults(defineProps<{ open: boolean; title: string; width?: string }>(), { width: 'sm:max-w-xl' })
const emit = defineEmits<{ (e: 'close'): void }>()

const titleId = `fb-sheet-title-${++sheetIdCounter}`
const panel = ref<HTMLElement>()
let locked = false
let previousActive: HTMLElement | null = null

// 挂在 document 上：焦点不在面板里（Safari 点按钮不聚焦、点了空白处）时 Esc 也能关。
// 已被别人处理过的 Esc（下拉框收起、图片预览关闭）不算
function onKeydown(e: KeyboardEvent) {
  if (props.open && e.key === 'Escape' && !e.defaultPrevented && !e.isComposing) emit('close')
}

function release() {
  if (!locked) return
  locked = false
  // 底下还有 BaseDialog 开着时由它负责解锁
  if (!document.querySelector('.modal-overlay')) document.body.classList.remove('modal-open')
  previousActive?.focus?.()
  previousActive = null
}

watch(
  () => props.open,
  async (open) => {
    if (!open) return release()
    if (!locked) previousActive = document.activeElement as HTMLElement | null
    locked = true
    document.body.classList.add('modal-open')
    await nextTick()
    // 面板里的内容自己没抢焦点时（例如手机上不自动弹键盘），先把焦点放进对话框
    if (panel.value && !panel.value.contains(document.activeElement)) panel.value.focus({ preventScroll: true })
  },
  { immediate: true }
)

onMounted(() => document.addEventListener('keydown', onKeydown))
onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown)
  release()
})
</script>

<style scoped>
/* 92dvh 跟随 iOS Safari 工具栏变化，不支持 dvh 的浏览器退回 92vh */
.fb-sheet-panel {
  max-height: 92vh;
  max-height: 92dvh;
}
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
