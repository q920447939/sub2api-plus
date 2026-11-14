<!-- [plus] 检测详情：某个账号的某项检测，按模型列出最近的每一次（小方块可点），可标为误报、再测一次 -->
<template>
  <BaseDialog :show="!!target" :title="data?.item.name ?? '检测详情'" width="wide" @close="emit('close')">
    <div v-if="!data" class="py-12 text-center text-sm text-gray-400">{{ error || '加载中…' }}</div>
    <div v-else data-testid="dispatch-probe">
      <div class="-mt-2 text-sm text-gray-500">{{ data.account }} · {{ data.spec.name }}</div>
      <div v-if="!data.models.length" class="py-10 text-center text-sm text-gray-400">还没有检测过</div>
      <div v-for="m in data.models" :key="m.model" class="mt-4 border-t border-gray-100 pt-4 dark:border-dark-700">
        <div class="flex flex-wrap items-center gap-2">
          <span class="font-mono text-sm font-semibold">{{ m.model }}</span>
          <Tag v-if="m.verdict" v-bind="verdictTag(m)" />
          <span class="ml-auto text-xs text-gray-500">{{ m.every ? everyText(m.every) : '没有定时检测' }}</span>
          <button v-if="canIgnore(m)" type="button" class="btn btn-secondary btn-sm" @click="toggleIgnore(m.model, !m.ignore)">{{ m.ignore ? '取消误报' : '标为误报' }}</button>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="busy" @click="retest([m.model])">再测一次</button>
        </div>
        <div v-if="m.verdict" class="mt-1 text-xs text-gray-500">{{ m.verdict.long }}<template v-if="m.ignore">；{{ m.ignore.operator || '有人' }} 在 {{ dt(m.ignore.created_at) }} 把之前的未通过标为误报</template></div>
        <div class="mt-2 flex flex-wrap gap-1">
          <button
            v-for="r in m.runs" :key="r.id" type="button" class="h-4 w-4 rounded-[5px]"
            :class="[DOT_CLASS[r.outcome] ?? 'bg-gray-300', sel[m.model] === r.id ? 'ring-2 ring-gray-800 ring-offset-2 dark:ring-white' : '']"
            :title="`${dt(r.queued_at)} · ${outcomeOf(data.spec, r.outcome).label}\n${r.error || r.summary}`" @click="sel[m.model] = r.id"
          />
        </div>
        <RunCard v-if="current(m)" :run="current(m)!" :spec="data.spec" class="mt-3" @open="emit('openRun', $event)" />
      </div>
      <template v-if="data.untested.length">
        <div class="mt-5 text-xs text-gray-400">没测过的模型</div>
        <div class="mt-2 flex flex-wrap gap-1.5">
          <button v-for="m in data.untested" :key="m" type="button" class="rounded-lg bg-gray-100 px-3 py-1 font-mono text-xs hover:bg-gray-200 dark:bg-dark-700" :disabled="busy" @click="retest([m])">{{ m }}</button>
        </div>
      </template>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type ProbeSheet } from '../api'
import { DOT_CLASS, dt, everyText, outcomeOf } from './format'
import RunCard from './RunCard.vue'
import Tag from './Tag.vue'

const props = defineProps<{ target: { accountId: number; itemId: number; model?: string } | null }>()
const emit = defineEmits<{ close: []; changed: []; openRun: [id: number] }>()
const appStore = useAppStore()
const data = ref<ProbeSheet | null>(null)
const error = ref('')
const busy = ref(false)
const sel = reactive<Record<string, number>>({})

async function load() {
  const t = props.target
  if (!t) return
  try {
    const d = await dispatchApi.probe(t.accountId, t.itemId)
    // 点进来的那个模型排第一
    if (t.model) d.models.sort((a, b) => Number(b.model === t.model) - Number(a.model === t.model))
    data.value = d
  } catch (e) {
    error.value = extractApiErrorMessage(e)
  }
}
watch(
  () => props.target,
  () => {
    data.value = null
    error.value = ''
    Object.keys(sel).forEach((k) => delete sel[k])
    void load()
  },
  { immediate: true }
)

type M = ProbeSheet['models'][number]
const current = (m: M) => m.runs.find((r) => r.id === sel[m.model]) ?? m.runs[m.runs.length - 1]
const canIgnore = (m: M) => !!m.ignore || m.verdict?.level === 'bad'

function verdictTag(m: M): { text: string; tone: 'green' | 'oamber' | 'gray' | 'red' } {
  const v = m.verdict!
  if (v.level === 'bad') return { text: '未通过', tone: 'oamber' }
  if (v.level === 'ok') return { text: v.ignored ? '通过（有误报）' : '通过', tone: 'green' }
  if (v.level === 'fault') return { text: '测不了', tone: 'red' }
  if (v.level === 'info') return { text: v.short, tone: 'gray' }
  return { text: '数据不足', tone: 'gray' }
}

async function retest(models: string[]) {
  const t = props.target!
  busy.value = true
  try {
    const r = await dispatchApi.retest(t.accountId, t.itemId, models)
    appStore.showSuccess(r.queued ? `已开始检测 ${models.join('、')}，结果出来后显示在这里` : '已经在排队了')
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e))
  } finally {
    busy.value = false
  }
}

async function toggleIgnore(model: string, on: boolean) {
  const t = props.target!
  try {
    await dispatchApi.ignore(t.accountId, t.itemId, model, on)
    appStore.showSuccess(on ? '已标为误报' : '已取消误报')
    await load()
    emit('changed')
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e))
  }
}
</script>
