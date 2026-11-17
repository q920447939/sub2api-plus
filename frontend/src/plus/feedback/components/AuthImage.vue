<!-- [plus] 鉴权图片：带 token 读取附件，点击全屏查看 -->
<template>
  <button
    ref="thumb"
    type="button"
    class="group relative block overflow-hidden rounded-xl bg-gray-100 dark:bg-dark-700"
    :class="sizeClass"
    aria-label="查看图片"
    @click="src && (zoom = true)"
  >
    <img v-if="src" :src="src" alt="" class="h-full w-full object-cover transition-transform group-hover:scale-105" />
    <span v-else-if="failed" class="flex h-full w-full items-center justify-center text-xs text-gray-400 dark:text-dark-400">失效</span>
  </button>
  <Teleport to="body">
    <div
      v-if="zoom"
      role="dialog"
      aria-modal="true"
      aria-label="图片预览"
      class="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 p-6 backdrop-blur-sm"
      @click="zoom = false"
    >
      <img :src="src" alt="" class="max-h-full max-w-full rounded-lg shadow-2xl" />
      <button
        ref="closeBtn"
        type="button"
        class="absolute right-4 top-4 flex h-9 w-9 items-center justify-center rounded-full bg-white/10 text-white hover:bg-white/20 focus:outline-none focus-visible:ring-2 focus-visible:ring-white/60"
        aria-label="关闭"
        @click.stop="zoom = false"
      >
        <Icon name="x" size="md" />
      </button>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { imageURL } from '../api'

const props = withDefaults(defineProps<{ id: number; admin?: boolean; size?: 'sm' | 'md' }>(), { admin: false, size: 'md' })

const src = ref('')
const failed = ref(false)
const zoom = ref(false)
const thumb = ref<HTMLButtonElement>()
const closeBtn = ref<HTMLButtonElement>()
const sizeClass = computed(() => (props.size === 'sm' ? 'h-16 w-16' : 'h-24 w-24'))

watch(
  () => [props.id, props.admin] as const,
  ([id, admin]) => {
    src.value = ''
    failed.value = false
    imageURL(id, admin)
      .then((u) => (src.value = u))
      .catch(() => (failed.value = true))
  },
  { immediate: true }
)

// 捕获阶段拦截：Esc 只关预览，不会再冒泡去关掉外面的反馈面板；Tab 留在关闭按钮上
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    zoom.value = false
  } else if (e.key === 'Tab') {
    e.preventDefault()
    closeBtn.value?.focus()
  }
}

watch(zoom, async (open) => {
  if (open) {
    document.addEventListener('keydown', onKeydown, true)
    await nextTick()
    closeBtn.value?.focus()
  } else {
    document.removeEventListener('keydown', onKeydown, true)
    thumb.value?.focus()
  }
})

onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown, true))
</script>
