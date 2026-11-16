<!-- [plus] 鉴权图片：带 token 读取附件，点击全屏查看 -->
<template>
  <button
    type="button"
    class="group relative block overflow-hidden rounded-xl bg-gray-100 dark:bg-dark-700"
    :class="sizeClass"
    @click="src && (zoom = true)"
  >
    <img v-if="src" :src="src" alt="" class="h-full w-full object-cover transition-transform group-hover:scale-105" />
    <span v-else-if="failed" class="flex h-full w-full items-center justify-center text-xs text-gray-400">失效</span>
  </button>
  <Teleport to="body">
    <div v-if="zoom" class="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 p-6 backdrop-blur-sm" @click="zoom = false">
      <img :src="src" alt="" class="max-h-full max-w-full rounded-lg shadow-2xl" />
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { imageURL } from '../api'

const props = withDefaults(defineProps<{ id: number; admin?: boolean; size?: 'sm' | 'md' }>(), { admin: false, size: 'md' })

const src = ref('')
const failed = ref(false)
const zoom = ref(false)
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
</script>
