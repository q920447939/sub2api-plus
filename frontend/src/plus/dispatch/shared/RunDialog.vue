<!-- [plus] 一次检测的全部内容：题面、回答（存档题显示作品：和智商检测同一个预览组件，走签名预览地址，脚本能跑） -->
<template>
  <BaseDialog :show="runId != null" title="检测内容" width="wide" :z-index="60" @close="emit('close')">
    <div v-if="!run" class="py-12 text-center text-sm text-gray-400">{{ error || '加载中…' }}</div>
    <div v-else class="space-y-3 text-sm">
      <div class="text-xs text-gray-500">{{ dt(run.queued_at) }} · <span class="font-mono">{{ run.model }}</span><span v-if="run.detail?.upstream_model"> → 实际发给上游 <span class="font-mono">{{ run.detail.upstream_model }}</span></span></div>
      <div v-if="run.summary" class="font-medium">{{ run.summary }}</div>
      <div v-if="run.error" class="break-all text-red-600">{{ run.error }}</div>
      <template v-if="run.outcome === 'saved' && run.preview_url">
        <PreviewFrame :key="run.id" :src="run.preview_url" class="rounded-lg border border-gray-200 dark:border-dark-700" />
        <div class="flex gap-4 text-xs">
          <a :href="buildApiUrl(run.preview_url)" target="_blank" rel="noopener noreferrer" class="text-primary-600 hover:underline">在新标签页打开</a>
          <a :href="buildApiUrl(run.preview_url) + '&download=1'" class="text-primary-600 hover:underline">下载 HTML</a>
        </div>
      </template>
      <template v-else>
        <div v-if="run.prompt"><div class="mb-1 text-xs text-gray-400">题面</div><pre class="max-h-56 overflow-auto whitespace-pre-wrap rounded-lg bg-gray-50 p-3 text-xs dark:bg-dark-800">{{ run.prompt }}</pre></div>
        <div v-if="run.reply"><div class="mb-1 text-xs text-gray-400">回答</div><pre class="max-h-80 overflow-auto whitespace-pre-wrap rounded-lg bg-gray-50 p-3 text-xs dark:bg-dark-800">{{ run.reply }}</pre></div>
      </template>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type Run } from '../api'
import { dt } from './format'
import { buildApiUrl } from '@/api/client'
import PreviewFrame from '@/plus/iq/shared/PreviewFrame.vue'

const props = defineProps<{ runId: number | null }>()
const emit = defineEmits<{ close: [] }>()
const run = ref<Run | null>(null)
const error = ref('')

watch(
  () => props.runId,
  async (id) => {
    run.value = null
    error.value = ''
    if (id == null) return
    try {
      run.value = await dispatchApi.run(id)
    } catch (e) {
      error.value = extractApiErrorMessage(e)
    }
  },
  { immediate: true }
)
</script>
