<!-- [plus] 图片选择：选中即上传，v-model 为附件 ID 列表；支持粘贴截图 -->
<template>
  <div v-if="max > 0" class="flex flex-wrap gap-2">
    <div v-for="(id, i) in modelValue" :key="id" class="relative">
      <AuthImage :id="id" :admin="admin" size="sm" />
      <button
        type="button"
        class="absolute -right-1.5 -top-1.5 flex h-5 w-5 items-center justify-center rounded-full bg-gray-900/80 text-xs text-white"
        aria-label="移除"
        @click="remove(i)"
      >
        ×
      </button>
    </div>
    <label
      v-if="modelValue.length + uploading < max"
      class="flex h-16 w-16 cursor-pointer items-center justify-center rounded-xl border border-dashed border-gray-300 text-gray-400 transition-colors hover:border-primary-400 hover:text-primary-500 dark:border-dark-600"
      :title="`最多 ${max} 张，每张不超过 ${maxMb}MB`"
    >
      <svg class="h-6 w-6" fill="none" stroke="currentColor" stroke-width="1.6" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" d="M2.25 15.75l5.16-5.16a2.25 2.25 0 013.18 0l5.16 5.16m-1.5-1.5l1.41-1.41a2.25 2.25 0 013.18 0l2.91 2.91M3.75 21h16.5A1.5 1.5 0 0021.75 19.5V4.5A1.5 1.5 0 0020.25 3H3.75A1.5 1.5 0 002.25 4.5v15A1.5 1.5 0 003.75 21z" />
      </svg>
      <input type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple class="hidden" @change="onPick" />
    </label>
    <div v-for="n in uploading" :key="'u' + n" class="flex h-16 w-16 items-center justify-center rounded-xl bg-gray-100 dark:bg-dark-700">
      <span class="h-4 w-4 animate-spin rounded-full border-2 border-gray-300 border-t-primary-500" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { uploadImage } from '../api'
import AuthImage from './AuthImage.vue'

const props = withDefaults(defineProps<{ modelValue: number[]; max: number; maxMb: number; admin?: boolean; paste?: boolean }>(), {
  admin: false,
  paste: false
})
const emit = defineEmits<{ (e: 'update:modelValue', v: number[]): void }>()
const appStore = useAppStore()
const uploading = ref(0)

async function upload(files: File[]) {
  const room = props.max - props.modelValue.length - uploading.value
  const list = files.filter((f) => f.type.startsWith('image/')).slice(0, Math.max(0, room))
  if (files.length > list.length && room <= 0) appStore.showError(`最多 ${props.max} 张`)
  const ids = [...props.modelValue]
  await Promise.all(
    list.map(async (f) => {
      if (f.size > props.maxMb * 1024 * 1024) {
        appStore.showError(`图片不能超过 ${props.maxMb}MB`)
        return
      }
      uploading.value++
      try {
        ids.push(await uploadImage(f, props.admin))
      } catch (e) {
        appStore.showError(extractApiErrorMessage(e, '上传失败'))
      } finally {
        uploading.value--
      }
    })
  )
  emit('update:modelValue', ids)
}

function onPick(e: Event) {
  const input = e.target as HTMLInputElement
  void upload(Array.from(input.files ?? []))
  input.value = ''
}

function remove(i: number) {
  const ids = [...props.modelValue]
  ids.splice(i, 1)
  emit('update:modelValue', ids)
}

function onPaste(e: ClipboardEvent) {
  const files = Array.from(e.clipboardData?.files ?? []).filter((f) => f.type.startsWith('image/'))
  if (files.length) {
    e.preventDefault()
    void upload(files)
  }
}

onMounted(() => props.paste && document.addEventListener('paste', onPaste))
onBeforeUnmount(() => document.removeEventListener('paste', onPaste))
</script>
