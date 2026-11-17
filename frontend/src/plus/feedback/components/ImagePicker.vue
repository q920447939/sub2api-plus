<!-- [plus] 图片选择：选中即上传，v-model 为附件 ID 列表；支持粘贴截图。v-model:uploading 告诉父组件还有几张在上传 -->
<template>
  <div v-if="max > 0" class="space-y-1.5">
    <div class="flex flex-wrap gap-2">
      <div v-for="(id, i) in modelValue" :key="id" class="relative">
        <AuthImage :id="id" :admin="admin" size="sm" />
        <button
          type="button"
          class="absolute -right-1.5 -top-1.5 flex h-5 w-5 items-center justify-center rounded-full bg-gray-900/80 text-xs text-white hover:bg-red-500"
          :aria-label="`移除第 ${i + 1} 张图片`"
          @click="remove(id)"
        >
          ×
        </button>
      </div>
      <div v-for="n in pending" :key="'u' + n" class="flex h-16 w-16 items-center justify-center rounded-xl bg-gray-100 dark:bg-dark-700" aria-label="上传中">
        <span class="h-4 w-4 animate-spin rounded-full border-2 border-gray-300 border-t-primary-500" />
      </div>
      <label
        v-if="modelValue.length + pending < max"
        class="flex h-16 w-16 cursor-pointer items-center justify-center rounded-xl border border-dashed border-gray-300 text-gray-400 transition-colors focus-within:border-primary-400 focus-within:ring-2 focus-within:ring-primary-400/40 hover:border-primary-400 hover:text-primary-500 dark:border-dark-600 dark:text-dark-400"
        :aria-label="`添加图片，最多 ${max} 张`"
      >
        <svg class="h-6 w-6" fill="none" stroke="currentColor" stroke-width="1.6" viewBox="0 0 24 24" aria-hidden="true">
          <path stroke-linecap="round" stroke-linejoin="round" d="M2.25 15.75l5.16-5.16a2.25 2.25 0 013.18 0l5.16 5.16m-1.5-1.5l1.41-1.41a2.25 2.25 0 013.18 0l2.91 2.91M3.75 21h16.5A1.5 1.5 0 0021.75 19.5V4.5A1.5 1.5 0 0020.25 3H3.75A1.5 1.5 0 002.25 4.5v15A1.5 1.5 0 003.75 21z" />
        </svg>
        <input type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple class="sr-only" data-testid="image-input" @change="onPick" />
      </label>
    </div>
    <p v-if="hint" class="text-xs text-gray-400 dark:text-dark-400">{{ hint }}</p>
  </div>
</template>

<script setup lang="ts">
import { getCurrentInstance, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { uploadImage } from '../api'
import AuthImage from './AuthImage.vue'

const props = withDefaults(
  defineProps<{
    modelValue: number[]
    max: number
    maxMb: number
    admin?: boolean
    paste?: boolean
    /** 选择框下方的一行小字，例如「最多 3 张 · 请勿包含 API Key」 */
    hint?: string
    /**
     * 组件已卸载（例如面板被关掉）时上传才完成，emit 会被 Vue 丢弃；
     * 父组件把草稿放在组件外时传这个回调，迟到的图片仍能写回草稿
     */
    lateAdd?: (id: number) => void
    /** 只为配合 v-model:uploading，组件不读取；实际数量由内部计数并通过 update:uploading 通知 */
    uploading?: number
  }>(),
  { admin: false, paste: false, hint: '', lateAdd: undefined, uploading: 0 }
)
const emit = defineEmits<{ (e: 'update:modelValue', v: number[]): void; (e: 'update:uploading', n: number): void }>()
const appStore = useAppStore()
const instance = getCurrentInstance()
const pending = ref(0) // 正在上传的张数
watch(pending, (n) => emit('update:uploading', n))

// 本地维护当前列表并同步更新：多张图在同一时刻上传完成时，props 还没来得及刷新，直接读 props 会互相覆盖
let current = [...props.modelValue]
watch(
  () => props.modelValue,
  (v) => (current = [...v])
)
function commit(next: number[]) {
  current = next
  emit('update:modelValue', next)
}

async function upload(files: File[]) {
  const images = files.filter((f) => f.type.startsWith('image/'))
  const room = Math.max(0, props.max - current.length - pending.value)
  const list = images.slice(0, room)
  const dropped = images.length - list.length
  if (dropped > 0) appStore.showError(room === 0 ? `最多 ${props.max} 张` : `最多 ${props.max} 张，已忽略 ${dropped} 张`)
  await Promise.all(
    list.map(async (f) => {
      if (f.size > props.maxMb * 1024 * 1024) {
        appStore.showError(`图片不能超过 ${props.maxMb}MB`)
        return
      }
      pending.value++
      try {
        const id = await uploadImage(f, props.admin)
        if (instance?.isUnmounted) props.lateAdd?.(id)
        else commit([...current, id])
      } catch (e) {
        appStore.showError(extractApiErrorMessage(e, '上传失败'))
      } finally {
        pending.value--
      }
    })
  )
}

function onPick(e: Event) {
  const input = e.target as HTMLInputElement
  void upload(Array.from(input.files ?? []))
  input.value = ''
}

function remove(id: number) {
  commit(current.filter((x) => x !== id))
}

function onPaste(e: ClipboardEvent) {
  const files = Array.from(e.clipboardData?.files ?? []).filter((f) => f.type.startsWith('image/'))
  if (files.length) {
    e.preventDefault()
    void upload(files)
  }
}

onMounted(() => props.paste && document.addEventListener('paste', onPaste))
onBeforeUnmount(() => {
  document.removeEventListener('paste', onPaste)
  if (pending.value) emit('update:uploading', 0)
})
</script>
