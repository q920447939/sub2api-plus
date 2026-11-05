<!-- [plus] 管理端检测详情：预览 + 状态、账号链路、耗时、Token、Key、请求 ID、原始输出 -->
<template>
  <BaseDialog :show="runId != null" :title="run ? `${run.group_name} · ${run.model}` : '检测详情'" width="full" @close="emit('close')">
    <div v-if="!run" class="p-10 text-center text-gray-400">{{ error || '正在加载…' }}</div>
    <template v-else>
      <div class="mb-3 flex items-center gap-2 text-sm">
        <span class="text-gray-500">{{ fullTime(run.finished_at || run.queued_at) }}</span>
        <span class="flex-1" />
        <template v-if="siblings.length > 1">
          <button type="button" class="btn btn-secondary btn-sm" :disabled="pos <= 0" @click="emit('navigate', siblings[pos - 1])">← 较新</button>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="pos < 0 || pos >= siblings.length - 1" @click="emit('navigate', siblings[pos + 1])">较旧 →</button>
        </template>
      </div>
      <div class="grid gap-5 lg:grid-cols-[1fr_300px]">
        <div>
          <PreviewFrame v-if="run.preview_url" :key="run.id" :src="run.preview_url" class="rounded-lg border border-gray-200 dark:border-dark-700" />
          <div
            v-else
            class="flex flex-col items-center justify-center gap-1 rounded-lg p-6 text-center text-sm text-red-500"
            :style="{ aspectRatio: '10 / 7', background: 'repeating-linear-gradient(45deg,#fafafa,#fafafa 8px,#f1f5f9 8px,#f1f5f9 16px)' }"
          >
            <b>{{ run.status === 'queued' ? '排队中' : run.status === 'running' ? '检测中…' : '✕ 检测失败' }}</b>
            <span class="text-gray-500">{{ run.error }}</span>
          </div>
          <div v-if="run.external_refs?.length" class="mt-2 rounded-lg bg-amber-50 p-2 text-xs text-amber-800">
            引用了外部资源（预览中已被拦截）：{{ run.external_refs.join('、') }}
          </div>
        </div>
        <div class="space-y-3 text-sm">
          <dl class="grid grid-cols-[76px_1fr] gap-x-2 gap-y-2">
            <dt class="text-gray-500">状态</dt>
            <dd><span class="rounded-full px-2 text-xs" :class="STATUS_CLASS[run.status]">{{ STATUS_LABEL[run.status] }}</span></dd>
            <dt class="text-gray-500">触发</dt>
            <dd>{{ run.trigger === 'cron' ? '定时' : '手动' }}</dd>
            <dt class="text-gray-500">账号链路</dt>
            <dd><AccountChain :chain="run.chain" :attributed="!!run.attributed_at" /></dd>
            <dt class="text-gray-500">耗时</dt>
            <dd>{{ run.duration_ms != null ? `${(run.duration_ms / 1000).toFixed(1)} s` : '—' }}</dd>
            <dt class="text-gray-500">Token</dt>
            <dd>输入 {{ tokens(run.input_tokens) }} / 输出 {{ tokens(run.output_tokens) }}</dd>
            <dt class="text-gray-500">探测 Key</dt>
            <dd class="break-all">{{ run.key_name || '—' }} <span class="font-mono text-xs text-gray-400">{{ run.key_masked }}</span></dd>
            <dt class="text-gray-500">请求 ID</dt>
            <dd class="break-all font-mono text-xs">{{ run.client_request_id || '—' }}</dd>
            <dt class="text-gray-500">题目版本</dt>
            <dd>v{{ run.prompt_version }}</dd>
          </dl>
          <div v-if="run.error && run.preview_url" class="text-xs text-red-500">{{ run.error }}</div>
          <div v-if="run.raw_excerpt">
            <div class="mb-1 text-xs text-gray-500">原始输出（节选）</div>
            <pre class="max-h-56 overflow-auto whitespace-pre-wrap rounded-lg bg-gray-900 p-2.5 text-[11px] text-gray-100">{{ run.raw_excerpt }}</pre>
          </div>
          <div class="flex flex-wrap gap-2">
            <a v-if="run.preview_url" class="btn btn-secondary btn-sm" :href="downloadUrl" download>下载 HTML</a>
            <a v-if="run.preview_url" class="btn btn-secondary btn-sm" :href="previewUrl" target="_blank" rel="noopener noreferrer">新窗口打开</a>
          </div>
        </div>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { buildApiUrl } from '@/api/client'
import { extractApiErrorMessage } from '@/utils/apiError'
import PreviewFrame from '../shared/PreviewFrame.vue'
import { fullTime, tokens } from '../shared/format'
import AccountChain from './AccountChain.vue'
import { getRun, type RunStatus, type RunView } from './api'

const props = withDefaults(defineProps<{ runId: number | null; siblings?: number[] }>(), { siblings: () => [] })
const emit = defineEmits<{ (e: 'close'): void; (e: 'navigate', id: number): void }>()

const STATUS_LABEL: Record<RunStatus, string> = { queued: '排队中', running: '检测中', success: '成功', failed: '失败' }
const STATUS_CLASS: Record<RunStatus, string> = {
  queued: 'bg-gray-100 text-gray-600',
  running: 'bg-blue-50 text-blue-600',
  success: 'bg-green-50 text-green-700',
  failed: 'bg-red-50 text-red-600'
}

const run = ref<RunView | null>(null)
const error = ref('')
const pos = computed(() => (props.runId == null ? -1 : props.siblings.indexOf(props.runId)))
const previewUrl = computed(() => (run.value?.preview_url ? buildApiUrl(run.value.preview_url) : ''))
const downloadUrl = computed(() => (previewUrl.value ? `${previewUrl.value}&download=1` : ''))

watch(
  () => props.runId,
  async (id) => {
    run.value = null
    error.value = ''
    if (id == null) return
    try {
      run.value = await getRun(id)
    } catch (e) {
      error.value = extractApiErrorMessage(e, '加载失败')
    }
  },
  { immediate: true }
)
</script>
