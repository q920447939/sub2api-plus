<!-- [plus] 上游调度台 · 操作记录：配置改动（改前 / 改后）、确认和误报、每次调整、档位切换 -->
<template>
  <AppLayout>
    <div class="space-y-4" data-testid="dispatch-audit">
      <div class="flex flex-wrap items-center gap-3">
        <div class="relative w-full sm:w-64">
          <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
          <input v-model="q.q" type="text" class="input pl-10" placeholder="搜索内容或操作人" @input="search" />
        </div>
        <PSelect v-model="q.target_type" width-class="w-36" :options="[{ value: '', label: '全部对象' }, ...Object.entries(TARGETS).map(([value, label]) => ({ value, label }))]" />
        <button type="button" class="btn btn-secondary ml-auto" title="刷新" @click="load"><Icon name="refresh" size="md" /></button>
      </div>
      <div class="card overflow-x-auto p-0">
        <table class="w-full text-sm">
          <thead><tr class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-800"><th class="px-3 py-2 font-normal">时间</th><th class="px-3 py-2 font-normal">操作人</th><th class="px-3 py-2 font-normal">对象</th><th class="px-3 py-2 font-normal">内容</th><th /></tr></thead>
          <tbody>
            <template v-for="e in items" :key="e.id">
              <tr class="border-t border-gray-100 dark:border-dark-700" data-testid="dispatch-audit-row">
                <td class="whitespace-nowrap px-3 py-2 text-xs">{{ fullTime(e.created_at) }}</td>
                <td class="px-3 py-2 text-xs">{{ e.operator || '系统' }}</td>
                <td class="px-3 py-2 text-xs">{{ TARGETS[e.target_type] ?? e.target_type }}</td>
                <td class="px-3 py-2">{{ e.summary }}</td>
                <td class="px-3 py-2 text-right"><button v-if="e.before || e.after" type="button" class="text-xs text-primary-700" @click="open[e.id] = !open[e.id]">{{ open[e.id] ? '收起' : '详情' }}</button></td>
              </tr>
              <tr v-if="open[e.id]"><td colspan="5" class="bg-gray-50 px-3 py-2 dark:bg-dark-900">
                <div class="grid gap-3 md:grid-cols-2">
                  <div><div class="mb-1 text-xs text-gray-500">改前</div><pre class="max-h-64 overflow-auto whitespace-pre-wrap break-all text-xs">{{ pretty(e.before) }}</pre></div>
                  <div><div class="mb-1 text-xs text-gray-500">改后</div><pre class="max-h-64 overflow-auto whitespace-pre-wrap break-all text-xs">{{ pretty(e.after) }}</pre></div>
                </div>
              </td></tr>
            </template>
            <tr v-if="!items.length"><td colspan="5" class="py-16 text-center text-sm text-gray-400">没有记录</td></tr>
          </tbody>
        </table>
        <div v-if="total > 50" class="flex items-center justify-center gap-3 p-3 text-sm">
          <button type="button" class="btn btn-secondary btn-sm" :disabled="page <= 1" @click="page--; load()">上一页</button>
          第 {{ page }} / {{ Math.ceil(total / 50) }} 页
          <button type="button" class="btn btn-secondary btn-sm" :disabled="page * 50 >= total" @click="page++; load()">下一页</button>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { dispatchApi, type AuditEntry } from '../api'
import Icon from '@/components/icons/Icon.vue'
import PSelect from '../shared/PSelect.vue'
import { fullTime } from '../shared/format'

const TARGETS: Record<string, string> = { config: '调度设置', dimension: '维度', item: '检测项', plan: '检测计划', run: '手动检测', account: '立即体检', diagnosis: '重新诊断', event: '待处理', adjustment: '调度调整' }
const q = reactive({ target_type: '', q: '' })
const page = ref(1)
const items = ref<AuditEntry[]>([])
const total = ref(0)
const open = reactive<Record<number, boolean>>({})
const pretty = (v: unknown) => (v == null ? '—' : JSON.stringify(v, null, 2))

async function load() {
  const r = await dispatchApi.audit({ ...q, page: page.value, page_size: 50 })
  items.value = r.items ?? []
  total.value = r.total
}
watch(() => q.target_type, () => { page.value = 1; void load() }, { immediate: true })
let timer: ReturnType<typeof setTimeout> | undefined
function search() {
  clearTimeout(timer)
  timer = setTimeout(() => { page.value = 1; void load() }, 300)
}
</script>
