<!-- [plus] 管理端「运行记录」：每次检测（包括失败）一条，带实际账号链路 -->
<template>
  <AppLayout>
    <div class="space-y-4" data-testid="iq-admin-runs">
      <header>
        <h1 class="text-xl font-bold text-gray-900 dark:text-white">运行记录</h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">
          每次检测都会记录，包括失败的。账号链路由请求 ID 反查用量日志和错误日志得到；用量日志可能晚几秒写入，查不到时显示「未知」。
        </p>
      </header>

      <div class="flex flex-wrap items-center gap-2">
        <select v-model.number="filter.group_id" class="input w-44" @change="reload">
          <option :value="0">全部分组</option>
          <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option>
        </select>
        <select v-model="filter.status" class="input w-32" @change="reload">
          <option value="">全部状态</option>
          <option value="success">成功</option>
          <option value="failed">失败</option>
          <option value="running">检测中</option>
          <option value="queued">排队中</option>
        </select>
        <select v-model="filter.trigger" class="input w-36" @change="reload">
          <option value="">全部触发方式</option>
          <option value="cron">定时</option>
          <option value="manual">手动</option>
        </select>
        <input v-model.number="accountInput" type="number" min="1" class="input w-36" placeholder="账号 ID" @keyup.enter="reload" />
        <button type="button" class="btn btn-secondary" @click="reload">查询</button>
        <span class="flex-1" />
        <span class="text-xs text-gray-500">共 {{ total }} 条</span>
      </div>

      <div class="card overflow-x-auto p-0">
        <table class="w-full text-sm">
          <thead class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-800">
            <tr>
              <th class="px-3 py-2.5">时间</th>
              <th class="px-3 py-2.5">分组</th>
              <th class="px-3 py-2.5">模型</th>
              <th class="px-3 py-2.5">预览</th>
              <th class="px-3 py-2.5">结果</th>
              <th class="px-3 py-2.5">实际账号链路</th>
              <th class="px-3 py-2.5">耗时</th>
              <th class="px-3 py-2.5">输出 Token</th>
              <th class="px-3 py-2.5">触发</th>
              <th class="px-3 py-2.5">请求 ID</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="r in rows"
              :key="r.id"
              class="cursor-pointer border-t border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-800"
              data-testid="iq-run-row"
              @click="detailId = r.id"
            >
              <td class="whitespace-nowrap px-3 py-2">{{ shortTime(r.finished_at || r.queued_at) }}</td>
              <td class="px-3 py-2">{{ r.group_name }}</td>
              <td class="px-3 py-2 font-mono text-xs">{{ r.model }}</td>
              <td class="px-3 py-2">
                <div class="w-14 overflow-hidden rounded">
                  <PreviewFrame v-if="r.preview_url" :src="r.preview_url" thumb />
                  <div v-else class="flex items-center justify-center bg-gray-100 text-[10px] text-gray-400" :style="{ aspectRatio: '10 / 7' }">—</div>
                </div>
              </td>
              <td class="px-3 py-2">
                <span class="rounded-full px-2 text-xs" :class="STATUS_CLASS[r.status]">{{ STATUS_LABEL[r.status] }}</span>
                <div v-if="r.error" class="mt-0.5 max-w-[220px] truncate text-xs text-gray-500" :title="r.error">{{ r.error }}</div>
              </td>
              <td class="px-3 py-2"><AccountChain v-if="r.status === 'success' || r.status === 'failed'" :chain="r.chain" :attributed="!!r.attributed_at" /></td>
              <td class="px-3 py-2">{{ seconds(r.duration_ms) }}</td>
              <td class="px-3 py-2">{{ tokens(r.output_tokens) }}</td>
              <td class="px-3 py-2">
                <span class="rounded-full px-2 text-xs" :class="r.trigger === 'cron' ? 'bg-gray-100 text-gray-600' : 'bg-amber-50 text-amber-700'">{{ r.trigger === 'cron' ? '定时' : '手动' }}</span>
              </td>
              <td class="px-3 py-2 font-mono text-xs text-gray-400" :title="r.client_request_id">{{ r.client_request_id ? `${r.client_request_id.slice(0, 13)}…` : '—' }}</td>
            </tr>
            <tr v-if="!rows.length">
              <td colspan="10" class="px-3 py-10 text-center text-gray-400">{{ loading ? '正在加载…' : '暂无记录' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <Pagination v-if="total > pageSize" :total="total" :page="page" :page-size="pageSize" :show-page-size-selector="false" @update:page="(p: number) => { page = p; load() }" />
    </div>
    <RunDetailDialog :run-id="detailId" :siblings="rows.map((r) => r.id)" @close="detailId = null" @navigate="(id) => (detailId = id)" />
  </AppLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import PreviewFrame from '../shared/PreviewFrame.vue'
import { seconds, shortTime, tokens } from '../shared/format'
import AccountChain from './AccountChain.vue'
import RunDetailDialog from './RunDetailDialog.vue'
import { getConfig, listRuns, type GroupInfo, type RunStatus, type RunView, type Trigger } from './api'

const STATUS_LABEL: Record<RunStatus, string> = { queued: '排队中', running: '检测中', success: '成功', failed: '失败' }
const STATUS_CLASS: Record<RunStatus, string> = {
  queued: 'bg-gray-100 text-gray-600',
  running: 'bg-blue-50 text-blue-600',
  success: 'bg-green-50 text-green-700',
  failed: 'bg-red-50 text-red-600'
}

const appStore = useAppStore()
const groups = ref<GroupInfo[]>([])
const filter = reactive<{ group_id: number; status: RunStatus | ''; trigger: Trigger | '' }>({ group_id: 0, status: '', trigger: '' })
const accountInput = ref<number | ''>('')
const rows = ref<RunView[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 30
const loading = ref(false)
const detailId = ref<number | null>(null)

async function load() {
  loading.value = true
  try {
    const res = await listRuns({ ...filter, account_id: Number(accountInput.value) || 0, page: page.value, page_size: pageSize })
    rows.value = res.items
    total.value = res.total
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '加载失败'))
  } finally {
    loading.value = false
  }
}

function reload() {
  page.value = 1
  void load()
}

let timer: ReturnType<typeof setInterval> | undefined
onMounted(async () => {
  void load()
  timer = setInterval(() => {
    if (detailId.value == null) void load()
  }, 15_000)
  try {
    groups.value = (await getConfig()).groups
  } catch {
    /* 筛选可选 */
  }
})
onBeforeUnmount(() => clearInterval(timer))
</script>
