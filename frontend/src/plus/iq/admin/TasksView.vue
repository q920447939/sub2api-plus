<!-- [plus] 管理端「检测任务」：按分组折叠（一个分组一块，下面列出该分组的模型） -->
<template>
  <AppLayout>
    <div class="space-y-4" data-testid="iq-admin-tasks">
      <header class="flex flex-wrap items-end gap-3">
        <div>
          <h1 class="text-xl font-bold text-gray-900 dark:text-white">检测任务</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">
            一条任务 = 分组 × 模型。定时检测由调度器执行，全局最多同时跑 {{ data?.stats.concurrency ?? '-' }} 个，其余排队。
          </p>
        </div>
        <span class="flex-1" />
        <button type="button" class="btn btn-secondary" @click="runAll">▶ 全部立即检测</button>
        <button type="button" class="btn btn-primary" data-testid="iq-task-new" @click="openEdit(null)">＋ 新增任务</button>
      </header>

      <div v-if="data && !data.stats.scheduler_on" class="rounded-lg bg-amber-50 px-3 py-2 text-sm text-amber-800">
        调度总开关已关闭，定时检测不会运行（手动检测仍可用）。可在「设置」中打开。
      </div>

      <div v-if="data" class="grid grid-cols-2 gap-3 md:grid-cols-4">
        <div class="card p-4"><div class="text-xl font-semibold">{{ data.stats.enabled_tasks }} / {{ data.stats.total_tasks }}</div><div class="text-xs text-gray-500">启用任务</div></div>
        <div class="card p-4"><div class="text-xl font-semibold">{{ data.stats.daily_runs }} 次</div><div class="text-xs text-gray-500">预计每日检测次数</div></div>
        <div class="card p-4"><div class="text-xl font-semibold">{{ data.stats.success_rate_24h == null ? '—' : `${Math.round(data.stats.success_rate_24h * 100)}%` }}</div><div class="text-xs text-gray-500">近 24 小时成功率</div></div>
        <div class="card p-4"><div class="text-xl font-semibold">{{ data.stats.running }} / {{ data.stats.concurrency }}</div><div class="text-xs text-gray-500">运行中 / 并发上限（排队 {{ data.stats.queued }}）</div></div>
      </div>

      <div v-if="!data" class="card p-10 text-center text-gray-400">{{ error || '正在加载…' }}</div>
      <div v-else-if="!grouped.length" class="card p-10 text-center text-gray-400">还没有检测任务，点击右上角「新增任务」。</div>

      <section v-for="g in grouped" :key="g.id" class="card overflow-hidden p-0" data-testid="iq-task-group-card">
        <div class="flex cursor-pointer flex-wrap items-center gap-2.5 px-4 py-3" @click="collapsed[g.id] = !collapsed[g.id]">
          <span class="w-3 text-gray-400">{{ collapsed[g.id] ? '▸' : '▾' }}</span>
          <b class="text-gray-900 dark:text-white">{{ g.name }}</b>
          <span class="rounded-full px-2 text-xs" :class="platformBadge(g.platform)">{{ g.platform }}</span>
          <span class="text-xs text-gray-500">{{ g.tasks.length }} 个模型 · {{ g.tasks.filter((t) => t.enabled).length }} 个启用</span>
          <span class="flex-1" />
          <span v-if="g.last" class="text-xs">
            <span class="rounded-full px-2" :class="g.last.status === 'success' ? 'bg-green-50 text-green-700' : 'bg-red-50 text-red-600'">{{ g.last.status === 'success' ? '成功' : '失败' }}</span>
            {{ shortTime(g.last.finished_at || g.last.queued_at) }}
          </span>
          <button type="button" class="btn btn-secondary btn-sm" @click.stop="run({ group_id: g.id })">▶ 整组检测</button>
          <button type="button" class="btn btn-secondary btn-sm" @click.stop="openEdit(null, g.id)">＋ 模型</button>
        </div>
        <div v-show="!collapsed[g.id]" class="border-t border-gray-100 bg-gray-50/60 px-4 py-1 dark:border-dark-700 dark:bg-dark-900/40">
          <div v-for="t in g.tasks" :key="t.id" class="flex flex-wrap items-center gap-3 border-b border-dashed border-gray-200 py-2.5 last:border-b-0 dark:border-dark-700" data-testid="iq-task-row">
            <span class="rounded-full border border-primary-200 bg-primary-50 px-2 font-mono text-xs text-primary-700">{{ t.model }}</span>
            <span class="min-w-[220px] text-xs">
              <span class="rounded-full px-1.5" :class="t.key_mode === 'auto' ? 'bg-primary-50 text-primary-700' : 'bg-gray-100 text-gray-600'">{{ t.key_mode === 'auto' ? '自动' : '指定' }}</span>
              {{ t.key?.name ?? `Key #${t.api_key_id}` }}
              <span class="font-mono text-gray-400">{{ t.key?.masked }}</span>
              <span v-if="t.key && !t.key.active" class="text-red-500">（不可用）</span>
            </span>
            <span class="text-xs text-gray-500">每 {{ t.interval_minutes }} 分钟 · 保留 {{ t.retention }}</span>
            <span class="text-xs text-gray-500">⏳ {{ t.enabled ? untilText(t.next_run_at) : '—' }}</span>
            <div class="flex gap-1">
              <button
                v-for="r in t.recent"
                :key="r.id"
                type="button"
                class="w-11 overflow-hidden rounded"
                :title="`${shortTime(r.finished_at || r.queued_at)} ${r.status === 'success' ? '成功' : r.error}`"
                @click="openRun(t.recent, r.id)"
              >
                <PreviewFrame v-if="r.preview_url" :src="r.preview_url" thumb />
                <div v-else class="flex items-center justify-center bg-red-50 text-[10px] text-red-500" :style="{ aspectRatio: '10 / 7' }">✕</div>
              </button>
            </div>
            <span v-if="t.active" class="rounded-full bg-blue-50 px-2 text-xs text-blue-600">{{ t.active === 'running' ? '检测中…' : '排队中' }}</span>
            <span class="flex-1" />
            <Toggle :model-value="t.enabled" @update:model-value="(v: boolean) => toggle(t, v)" />
            <button type="button" class="btn btn-secondary btn-sm" @click="run({ task_id: t.id })">▶ 检测</button>
            <button type="button" class="btn btn-secondary btn-sm" @click="openEdit(t)">编辑</button>
            <button type="button" class="btn btn-danger btn-sm" @click="pendingDelete = t">删除</button>
          </div>
        </div>
      </section>
    </div>

    <TaskDialog
      :open="editOpen"
      :task="editing"
      :groups="groups"
      :preset-group-id="presetGroup"
      :defaults="defaults"
      @close="editOpen = false"
      @saved="editOpen = false; load()"
    />
    <RunDetailDialog :run-id="detailId" :siblings="siblings" @close="detailId = null" @navigate="(id) => (detailId = id)" />
    <ConfirmDialog
      :show="!!pendingDelete"
      title="删除检测任务"
      :message="pendingDelete ? `确定删除「${pendingDelete.group_name} · ${pendingDelete.model}」？该任务的所有检测记录会一起删除。` : ''"
      confirm-text="删除"
      danger
      @confirm="confirmDelete"
      @cancel="pendingDelete = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import PreviewFrame from '../shared/PreviewFrame.vue'
import { platformBadge, shortTime, untilText } from '../shared/format'
import RunDetailDialog from './RunDetailDialog.vue'
import TaskDialog from './TaskDialog.vue'
import {
  deleteTask, getConfig, getTasks, runNow, updateTask,
  type GroupInfo, type RunView, type TasksOverview, type TaskView
} from './api'

const appStore = useAppStore()
const data = ref<TasksOverview | null>(null)
const groups = ref<GroupInfo[]>([])
const defaults = reactive({ interval: 30, retention: 48 })
const error = ref('')
const collapsed = reactive<Record<number, boolean>>({})
const editOpen = ref(false)
const editing = ref<TaskView | null>(null)
const presetGroup = ref<number | undefined>()
const pendingDelete = ref<TaskView | null>(null)
const detailId = ref<number | null>(null)
const siblings = ref<number[]>([])

const grouped = computed(() => {
  const map = new Map<number, { id: number; name: string; platform: string; tasks: TaskView[]; last?: RunView }>()
  for (const t of data.value?.tasks ?? []) {
    const g = map.get(t.group_id) ?? { id: t.group_id, name: t.group_name, platform: t.platform, tasks: [] }
    g.tasks.push(t)
    const r = t.recent[0]
    if (r && (!g.last || r.id > g.last.id)) g.last = r
    map.set(t.group_id, g)
  }
  return [...map.values()]
})

async function load() {
  try {
    data.value = await getTasks()
  } catch (e) {
    error.value = extractApiErrorMessage(e, '加载失败')
  }
}

function openEdit(t: TaskView | null, groupId?: number) {
  editing.value = t
  presetGroup.value = groupId
  editOpen.value = true
}

function openRun(runs: RunView[], id: number) {
  siblings.value = runs.map((r) => r.id)
  detailId.value = id
}

async function run(target: { task_id?: number; group_id?: number }) {
  try {
    const r = await runNow(target)
    appStore.showSuccess(r.queued ? `已加入队列 ${r.queued} 个${r.skipped ? `，${r.skipped} 个已在排队中` : ''}` : '任务已在排队或运行中')
    void load()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '操作失败'))
  }
}

const runAll = () => run({})

async function toggle(t: TaskView, enabled: boolean) {
  try {
    await updateTask(t.id, {
      group_id: t.group_id, model: t.model, key_mode: t.key_mode, api_key_id: t.api_key_id, format: t.format,
      thinking: t.thinking, interval_minutes: t.interval_minutes, retention: t.retention, enabled
    })
    t.enabled = enabled
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '操作失败'))
  }
}

async function confirmDelete() {
  const t = pendingDelete.value
  pendingDelete.value = null
  if (!t) return
  try {
    await deleteTask(t.id)
    appStore.showSuccess('已删除')
    void load()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '删除失败'))
  }
}

let timer: ReturnType<typeof setInterval> | undefined
onMounted(async () => {
  void load()
  timer = setInterval(load, 15_000)
  try {
    const cfg = await getConfig()
    groups.value = cfg.groups
    defaults.interval = cfg.config.default_interval
    defaults.retention = cfg.config.default_retention
  } catch {
    /* 新建任务时再提示 */
  }
})
onBeforeUnmount(() => clearInterval(timer))
</script>
