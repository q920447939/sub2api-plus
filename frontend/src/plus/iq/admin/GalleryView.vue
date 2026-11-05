<!--
  [plus] 管理端「检测画廊」：与用户端同样的横排布局，额外显示失败记录、已停用任务、耗时和实际命中的账号。
  这是独立的管理员页面（菜单即身份），不与用户页共享路由或接口。
-->
<template>
  <AppLayout>
    <div class="space-y-4" data-testid="iq-admin-gallery">
      <header class="flex flex-wrap items-end gap-3">
        <div>
          <h1 class="text-xl font-bold text-gray-900 dark:text-white">检测画廊</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">
            包含失败记录和已停用的任务。卡片下方是耗时和实际命中的账号（→ 表示发生了故障切换），点开可以看完整账号链路。
          </p>
        </div>
        <span class="flex-1" />
        <select v-model.number="groupId" class="input w-48" @change="load">
          <option :value="0">全部分组</option>
          <option v-for="g in allGroups" :key="g.id" :value="g.id">{{ g.name }}</option>
        </select>
        <button type="button" class="btn btn-secondary" @click="load">刷新</button>
        <router-link :to="`${IQ_ADMIN_BASE}/runs`" class="btn btn-secondary">运行记录 →</router-link>
      </header>

      <div v-if="!rows" class="card p-10 text-center text-gray-400">{{ error || '正在加载…' }}</div>
      <div v-else-if="!rows.length" class="card p-10 text-center text-gray-400">
        还没有检测任务，先去 <router-link :to="`${IQ_ADMIN_BASE}/tasks`" class="text-primary-600">检测任务</router-link> 新建一个。
      </div>

      <section v-for="g in rows ?? []" :key="g.id" class="card flex flex-col overflow-hidden p-0 md:flex-row" data-testid="iq-admin-row">
        <div class="flex-none border-b border-gray-100 bg-gradient-to-b from-white to-gray-50 p-4 text-xs text-gray-500 dark:border-dark-700 dark:from-dark-800 dark:to-dark-900 dark:text-dark-300 md:w-60 md:border-b-0 md:border-r">
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ g.name }}</h3>
          <div class="mt-1.5 flex flex-wrap gap-1">
            <span class="rounded-full px-2" :class="platformBadge(g.platform)">{{ g.platform }}</span>
            <span v-if="g.tasks.every((t) => !t.enabled)" class="rounded-full bg-gray-100 px-2 text-gray-500">已停用</span>
          </div>
          <div class="mt-2 space-y-1">
            <div>模型：<span class="font-mono">{{ g.tasks.map((t) => t.model).join('、') }}</span></div>
            <div>频率：每 {{ g.tasks[0].interval_minutes }} 分钟</div>
            <div>⏳ 下次：{{ g.tasks[0].enabled ? untilText(g.tasks[0].next_run_at) : '—' }}</div>
            <div>账号池：{{ g.account_count }} 个</div>
          </div>
          <button type="button" class="btn btn-secondary btn-sm mt-3" :disabled="busy[g.id]" @click="runGroup(g.id)">
            {{ busy[g.id] ? '已加入队列' : '▶ 立即检测' }}
          </button>
        </div>
        <div class="min-w-0 flex-1 space-y-3 py-3 pl-3">
          <div v-for="t in g.tasks" :key="t.task_id">
            <span
              v-if="g.tasks.length > 1"
              class="mb-1.5 inline-block rounded-full border border-primary-200 bg-primary-50 px-2 text-xs text-primary-700"
            >{{ t.model }}<template v-if="!t.enabled">（已停用）</template></span>
            <SampleStrip :items="t.runs.map(toItem)" @open="(i) => openRun(t.runs, i)">
              <template #meta="{ index }">
                <span class="font-medium">{{ shortTime(t.runs[index].finished_at || t.runs[index].queued_at) }}</span>
                <span class="text-gray-400">{{ seconds(t.runs[index].duration_ms) }}</span>
                <AccountChain :chain="t.runs[index].chain.slice(-1)" :attributed="!!t.runs[index].attributed_at" compact />
                <span v-if="t.runs[index].chain.length > 1" class="text-gray-400" title="发生了故障切换">↪</span>
              </template>
            </SampleStrip>
          </div>
        </div>
      </section>
    </div>
    <RunDetailDialog :run-id="detailId" :siblings="siblings" @close="detailId = null" @navigate="(id) => (detailId = id)" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import SampleStrip, { type StripItem } from '../shared/SampleStrip.vue'
import { platformBadge, seconds, shortTime, untilText } from '../shared/format'
import { IQ_ADMIN_BASE } from '../routes'
import AccountChain from './AccountChain.vue'
import RunDetailDialog from './RunDetailDialog.vue'
import { getConfig, getGallery, runNow, type AdminGalleryGroup, type GroupInfo, type RunView } from './api'

const appStore = useAppStore()
const rows = ref<AdminGalleryGroup[] | null>(null)
const allGroups = ref<GroupInfo[]>([])
const groupId = ref(0)
const error = ref('')
const busy = reactive<Record<number, boolean>>({})
const detailId = ref<number | null>(null)
const siblingRuns = ref<RunView[]>([])
const siblings = computed(() => siblingRuns.value.map((r) => r.id))

function toItem(r: RunView): StripItem {
  return { key: r.id, previewUrl: r.preview_url, failText: r.error }
}

function openRun(runs: RunView[], i: number) {
  siblingRuns.value = runs
  detailId.value = runs[i].id
}

async function load() {
  error.value = ''
  try {
    rows.value = await getGallery(groupId.value || undefined)
  } catch (e) {
    error.value = extractApiErrorMessage(e, '加载失败')
  }
}

async function runGroup(id: number) {
  busy[id] = true
  try {
    const r = await runNow({ group_id: id })
    appStore.showSuccess(r.queued ? `已加入队列 ${r.queued} 个` : '该分组的任务已在排队或运行中')
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '操作失败'))
  } finally {
    setTimeout(() => (busy[id] = false), 1500)
  }
}

onMounted(async () => {
  void load()
  try {
    allGroups.value = (await getConfig()).groups
  } catch {
    /* 分组筛选可选，失败不影响画廊 */
  }
})
</script>
