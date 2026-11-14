<!-- [plus] 上游调度台 · 操作日志：谁、什么时候、改了什么（写清前 → 后），按类型筛选、搜索 -->
<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3" data-testid="dispatch-audit">
          <div class="inline-flex rounded-lg bg-gray-100 p-0.5 dark:bg-dark-700">
            <button v-for="k in KINDS" :key="k.k" type="button" class="rounded-md px-3 py-1 text-sm" :class="kind === k.k ? 'bg-white font-medium shadow-sm dark:bg-dark-900' : 'text-gray-500'" @click="kind = k.k; page = 1">
              {{ k.n }}<span class="ml-1 text-xs text-gray-400">{{ k.k ? counts[k.k] ?? 0 : total0 }}</span>
            </button>
          </div>
          <div class="relative ml-auto w-64">
            <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input v-model="kw" class="input pl-10" placeholder="搜索内容或操作人" />
          </div>
        </div>
      </template>
      <template #table>
        <table class="table">
          <thead><tr><th class="w-36">时间</th><th class="w-44">操作人</th><th class="w-20">类型</th><th>内容</th></tr></thead>
          <tbody>
            <tr v-for="x in list" :key="x.id">
              <td class="whitespace-nowrap text-sm tabular-nums text-gray-500">{{ dt(x.created_at) }}</td>
              <td class="whitespace-nowrap">{{ x.operator || '系统' }}</td>
              <td><span class="badge" :class="KIND_CLASS[x.target_type]">{{ KIND_NAME[x.target_type] }}</span></td>
              <td class="text-sm">{{ x.summary }}</td>
            </tr>
            <tr v-if="!list.length"><td colspan="4" class="py-14 text-center text-sm text-gray-400">{{ error || '没有记录' }}</td></tr>
          </tbody>
        </table>
      </template>
      <template #pagination>
        <Pagination v-if="total > pageSize" :total="total" :page="page" :page-size="pageSize" @update:page="page = $event" @update:page-size="pageSize = $event; page = 1" />
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type AuditEntry } from '../api'
import { dt } from '../shared/format'

const KINDS = [{ k: '', n: '全部' }, { k: 'weight', n: '权重' }, { k: 'detect', n: '检测' }, { k: 'config', n: '配置' }]
const KIND_NAME: Record<string, string> = { weight: '权重', detect: '检测', config: '配置' }
const KIND_CLASS: Record<string, string> = { weight: 'badge-primary', detect: 'badge-success', config: 'badge-gray' }

const list = ref<AuditEntry[]>([])
const total = ref(0)
const counts = ref<Record<string, number>>({})
const error = ref('')
const kind = ref('')
const kw = ref('')
const q = ref('')
const page = ref(1)
const pageSize = ref(50)
const total0 = computed(() => Object.values(counts.value).reduce((s, n) => s + n, 0))

async function load() {
  try {
    const r = await dispatchApi.audit({ kind: kind.value, q: q.value, page: page.value, page_size: pageSize.value })
    list.value = r.list
    total.value = r.total
    counts.value = r.counts ?? {}
    error.value = ''
  } catch (e) {
    error.value = extractApiErrorMessage(e)
  }
}
let timer: ReturnType<typeof setTimeout> | undefined
watch(kw, (v) => {
  clearTimeout(timer)
  timer = setTimeout(() => {
    q.value = v.trim()
    page.value = 1
  }, 300)
})
watch([kind, q, page, pageSize], load, { immediate: true })
</script>
