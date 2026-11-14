<!-- [plus] 上游调度台 · 调整记录：生效中的设置（一行一条，可恢复正常）+ 历史（每次调整一条，展开看前后对比） -->
<template>
  <AppLayout>
    <div class="space-y-4" data-testid="dispatch-changes">
      <div class="flex flex-wrap items-center gap-3">
        <div class="inline-flex rounded-lg bg-gray-100 p-0.5 dark:bg-dark-700">
          <button v-for="w in WINDOWS" :key="w.k" type="button" class="rounded-md px-3 py-1 text-sm" :class="win === w.k ? 'bg-white font-medium shadow-sm dark:bg-dark-900' : 'text-gray-500'" @click="win = w.k">{{ w.n }}</button>
        </div>
        <div class="ml-auto flex items-center gap-2 text-sm text-gray-500">
          权重设置生效
          <Toggle :model-value="data?.apply ?? true" @update:model-value="setApply" />
        </div>
        <button type="button" class="btn btn-secondary" :disabled="loading" title="刷新" @click="load"><Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" /></button>
      </div>
      <ApplyOff v-if="data && !data.apply" :rules="data.active.length" @changed="load" />
      <div v-if="!data" class="py-16 text-center text-sm text-gray-400">{{ error || '加载中…' }}</div>
      <template v-else>
        <div class="px-1 text-sm text-gray-500">{{ data.apply ? '生效中' : '已设置，没有生效' }} {{ data.active.length }}</div>
        <div class="card overflow-x-auto p-0">
          <table v-if="data.active.length" class="table">
            <thead><tr><th>账号</th><th>分组</th><th>模型</th><th>设置</th><th>谁 · 什么时候 · 为什么</th><th></th></tr></thead>
            <tbody>
              <tr v-for="r in data.active" :key="`${r.account_id}-${r.group_id}-${r.model}`" data-testid="dispatch-active-rule">
                <td class="whitespace-nowrap font-semibold"><router-link :to="`${DISPATCH_BASE}/account/${r.account_id}`" class="hover:underline">{{ aname(r.account_id) }}</router-link></td>
                <td class="whitespace-nowrap">{{ gname(r.group_id) }}</td>
                <td :class="r.model ? 'font-mono text-sm' : 'text-gray-500'">{{ r.model || '全部模型' }}</td>
                <td><FlowButton :rules="[r]" :groups="1" :model="r.model" @click="weight = { accountId: r.account_id, model: r.model }" /></td>
                <td class="max-w-[360px] truncate text-xs text-gray-500">{{ [r.operator?.split('@')[0], dt(r.created_at), r.note].filter(Boolean).join(' · ') }}</td>
                <td class="text-right"><button type="button" class="btn btn-secondary btn-sm" @click="restore(r)">恢复正常</button></td>
              </tr>
            </tbody>
          </table>
          <div v-else class="py-8 text-center text-sm text-gray-400">所有账号都是正常</div>
        </div>
        <div class="px-1 pt-2 text-sm text-gray-500">历史</div>
        <div class="card overflow-hidden p-0">
          <RecordCard v-for="(r, i) in data.records" :key="r.id" :rec="r" :names="data.names" :start-open="i === 0" @undo="undo" @move-away="moveAway" />
          <div v-if="!data.records.length" class="py-10 text-center text-sm text-gray-400">还没有调整</div>
        </div>
      </template>
    </div>
    <WeightDialog :target="weight" @close="weight = null" @saved="load" />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type Changes, type Rule } from '../api'
import { DISPATCH_BASE } from '../routes'
import ApplyOff from '../shared/ApplyOff.vue'
import FlowButton from '../shared/FlowButton.vue'
import RecordCard from '../shared/RecordCard.vue'
import WeightDialog, { type WeightTarget } from '../shared/WeightDialog.vue'
import { dt } from '../shared/format'

const WINDOWS = [{ k: 'auto', n: '调整后至今' }, { k: '1', n: '1 小时' }, { k: '6', n: '6 小时' }, { k: '24', n: '24 小时' }]
const appStore = useAppStore()
const data = ref<Changes | null>(null)
const error = ref('')
const loading = ref(false)
const win = ref('auto')
const weight = ref<WeightTarget | null>(null)

const aname = (id: number) => data.value?.names.accounts[String(id)] ?? `#${id}`
const gname = (id: number) => data.value?.names.groups[String(id)] ?? `#${id}`

async function load() {
  loading.value = true
  try {
    data.value = await dispatchApi.changes(win.value)
    error.value = ''
  } catch (e) {
    error.value = extractApiErrorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(win, load, { immediate: true })

async function act(fn: () => Promise<unknown>, ok: string) {
  try {
    await fn()
    appStore.showSuccess(ok)
    await load()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e))
  }
}

const restore = (r: Rule) => act(() => dispatchApi.restore(r.account_id, r.group_id, r.model), '已恢复正常')
const undo = (id: number) => act(() => dispatchApi.undo(id), '已恢复原样')
async function setApply(on: boolean) {
  await act(async () => {
    const v = await dispatchApi.config()
    await dispatchApi.saveConfig({ ...v.config, apply: on }, v.version)
  }, on ? '权重设置已生效' : '权重设置已暂停生效，设置都留着')
}
function moveAway(p: { accountId: number; models: string[] }) {
  weight.value = { accountId: p.accountId, model: p.models[0], also: p.models.slice(1), note: '这些模型在它身上全部失败', pauseAll: true }
}
</script>
