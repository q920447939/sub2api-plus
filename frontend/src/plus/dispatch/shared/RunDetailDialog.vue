<!-- [plus] 一次执行的详情：发了什么、回了什么、怎么判的、用的哪个账号。展示方式按检测方式选。 -->
<template>
  <BaseDialog :show="runId != null" :title="data ? `检测详情 #${data.run.id}` : '检测详情'" width="extra-wide" @close="emit('close')">
    <div v-if="!data" class="py-16 text-center text-sm text-gray-400">{{ error || '加载中…' }}</div>
    <div v-else class="space-y-4 text-sm" data-testid="dispatch-run-detail">
      <div class="flex flex-wrap items-center gap-2">
        <span class="rounded-md border px-2 py-0.5 text-xs" :class="TONE_CLASS[oc.tone]">{{ oc.label }}</span>
        <span class="font-medium">{{ data.run.summary }}</span>
      </div>
      <dl class="grid grid-cols-[96px_1fr] gap-x-3 gap-y-1.5">
        <dt class="text-gray-500">时间</dt><dd>{{ fullTime(data.run.queued_at) }}（{{ agoText(data.run.queued_at) }}）· {{ TRIGGER[data.run.trigger] ?? data.run.trigger }}</dd>
        <dt class="text-gray-500">检测项</dt><dd>{{ data.item?.name }}</dd>
        <dt class="text-gray-500">计划</dt><dd>{{ data.plan ? `${platformName(data.plan.platform)} · ${everyText(data.plan.interval_minutes)}` : '—' }}</dd>
        <dt class="text-gray-500">账号</dt><dd>{{ data.account?.name ?? '#' + data.run.account_id }} <span class="text-xs text-gray-500">{{ (data.account?.groups ?? []).join('、') }}</span></dd>
        <dt class="text-gray-500">模型</dt><dd class="font-mono text-xs">{{ data.run.model }}<span v-if="data.run.detail?.upstream_model && data.run.detail.upstream_model !== data.run.model" class="text-gray-400"> → {{ data.run.detail.upstream_model }}</span></dd>
        <dt class="text-gray-500">地址</dt><dd class="font-mono text-xs">{{ data.account?.base_url }}</dd>
        <dt class="text-gray-500">耗时 / Token</dt><dd>{{ data.run.duration_ms != null ? (data.run.duration_ms / 1000).toFixed(1) + ' 秒' : '—' }} · 输入 {{ data.run.input_tokens ?? '—' }} / 输出 {{ data.run.output_tokens ?? '—' }}</dd>
      </dl>

      <div v-if="data.run.error" class="rounded-lg bg-slate-100 px-3 py-2 text-xs text-slate-700 dark:bg-dark-700 dark:text-dark-100">
        {{ data.run.error }}
      </div>

      <!-- 指纹：候选分布 -->
      <div v-if="data.item?.type === 'fingerprint' && data.run.detail?.top" class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
        <div class="mb-2 text-xs text-gray-500">最接近的模型</div>
        <div v-for="t in data.run.detail.top" :key="t.model" class="flex items-center gap-2 text-xs">
          <span class="w-56 truncate font-mono" :class="norm(t.model) === norm(data.run.detail.declared) ? 'text-green-700' : ''">{{ t.model }}</span>
          <span class="h-2 flex-1 overflow-hidden rounded bg-gray-100 dark:bg-dark-700"><i class="block h-full bg-primary-500" :style="{ width: t.probability + '%' }" /></span>
          <span class="w-12 text-right">{{ t.probability }}%</span>
        </div>
      </div>
      <!-- 判分题：答案 -->
      <div v-if="data.item?.type === 'graded' && data.run.detail" class="rounded-lg border border-gray-200 p-3 text-xs dark:border-dark-700">
        提取到 <b>{{ data.run.detail.answer || '—' }}</b> · 标准答案 <b>{{ (data.run.detail.expected ?? []).join(' / ') }}</b>
      </div>
      <!-- 存档题：作品 -->
      <div v-if="data.item?.type === 'archive' && data.run.outcome === 'saved'">
        <HtmlPreview :html="data.run.reply" />
      </div>

      <div>
        <div class="mb-1 text-xs font-medium text-gray-500">题面</div>
        <pre class="max-h-56 overflow-auto whitespace-pre-wrap break-all rounded-lg border border-gray-200 bg-gray-50 p-3 text-xs dark:border-dark-700 dark:bg-dark-800">{{ data.run.prompt || '—' }}</pre>
      </div>
      <div>
        <div class="mb-1 text-xs font-medium text-gray-500">回答</div>
        <pre class="max-h-72 overflow-auto whitespace-pre-wrap break-all rounded-lg border border-gray-200 bg-gray-50 p-3 text-xs dark:border-dark-700 dark:bg-dark-800">{{ data.run.reply || '—' }}</pre>
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end gap-2">
        <button v-if="data" type="button" class="btn btn-secondary" :disabled="busy" @click="rerun">重测</button>
        <button type="button" class="btn btn-primary" @click="emit('close')">关闭</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi } from '../api'
import { agoText, everyText, fullTime, outcomeOf, platformName, TONE_CLASS } from './format'
import HtmlPreview from './HtmlPreview.vue'

const TRIGGER: Record<string, string> = { cron: '定时', manual: '手动', checkup: '立即体检' }

const props = defineProps<{ runId: number | null }>()
const emit = defineEmits<{ close: []; rerun: [] }>()
const app = useAppStore()
const data = ref<any>(null)
const error = ref('')
const busy = ref(false)
const oc = computed(() => outcomeOf(data.value?.spec, data.value?.outcome ?? data.value?.run?.outcome))
const norm = (m: string) => (m ?? '').toLowerCase().replace(/[._]/g, '-')

watch(
  () => props.runId,
  async (id) => {
    data.value = null
    error.value = ''
    if (id == null) return
    try {
      data.value = await dispatchApi.run(id)
    } catch (e) {
      error.value = extractApiErrorMessage(e, '加载失败')
    }
  },
  { immediate: true }
)

async function rerun() {
  busy.value = true
  try {
    await dispatchApi.rerun(data.value.run.id)
    app.showSuccess('已排队重测，结果会出现在检测记录里')
    emit('rerun')
  } catch (e) {
    app.showError(extractApiErrorMessage(e, '重测失败'))
  } finally {
    busy.value = false
  }
}
</script>
