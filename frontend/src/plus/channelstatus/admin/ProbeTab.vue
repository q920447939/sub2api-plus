<template>
  <div v-if="p" class="space-y-5" data-testid="cs-probe-tab">
    <section class="card space-y-4 p-5">
      <div class="flex items-start justify-between gap-4">
        <div>
          <h3 class="font-semibold text-gray-900 dark:text-white">兜底探测</h3>
          <p class="text-xs text-gray-400">
            只在<strong>有人打开渠道状态页</strong>、且分组在判定窗口内<strong>真实请求数不足最小样本</strong>时才触发，有真实流量时不探测。
            探测走本站网关、用该分组下的 Key 发一条极短的算术题请求（与上游 V1 相同），结果计入当前状态，不计入历史条。
          </p>
        </div>
        <Toggle v-model="p.enabled" />
      </div>

      <label class="block text-sm text-gray-600 dark:text-dark-300">
        网关地址
        <div class="mt-1 flex gap-2">
          <input v-model.trim="p.endpoint" class="input flex-1" placeholder="https://api.example.com" />
          <button type="button" class="btn btn-secondary whitespace-nowrap" @click="p.endpoint = origin">使用当前域名</button>
        </div>
        <span class="mt-1 block text-xs text-gray-400">从本服务所在机器能访问到的网关地址即可，支持内网 IP（如 http://192.168.1.10:8090）；容器内 localhost 指容器自身。探测不跟随重定向</span>
      </label>

      <div class="grid gap-4 md:grid-cols-4">
        <NumField v-model="p.fresh_minutes" label="成功结果保质期" suffix="分钟" :min="5" :max="720" />
        <NumField v-model="p.retry_minutes" label="失败后复测间隔" suffix="分钟" :min="1" :max="120" />
        <NumField v-model="p.base_cooldown_minutes" label="无流量冷却起点" suffix="分钟" :min="1" :max="720" />
        <NumField v-model="p.max_cooldown_hours" label="冷却上限" suffix="小时" :min="1" :max="48" />
        <NumField v-model="p.daily_limit" label="每分组每日上限" suffix="次" :min="1" :max="1440" />
        <NumField v-model="p.timeout_seconds" label="单次探测超时" suffix="秒" :min="5" :max="120" />
        <NumField v-model="p.concurrency" label="并发上限（每实例）" :min="1" :max="16" />
      </div>
      <p class="text-xs text-gray-400">
        成功结果在保质期内直接使用，过期后显示「暂无数据」（不会显示绿色）；连续无真实流量时冷却按 起点 × 2ⁿ 拉长到上限，一出现真实流量立即回到起点。
        失败结果保持「异常」并按复测间隔重试。
      </p>
    </section>

    <section class="card overflow-x-auto p-0">
      <table class="w-full min-w-[880px] table-fixed text-sm">
        <colgroup>
          <col class="w-16" />
          <col class="w-36" />
          <col />
          <col class="w-40" />
          <col />
          <col class="w-44" />
          <col class="w-28" />
        </colgroup>
        <thead class="bg-gray-50 text-left text-xs text-gray-400 dark:bg-dark-800">
          <tr>
            <th class="px-3 py-2">启用</th>
            <th class="px-3 py-2">平台 / 分组</th>
            <th class="px-3 py-2">探测 Key（我的 Key）</th>
            <th class="px-3 py-2">请求格式</th>
            <th class="px-3 py-2">探测模型</th>
            <th class="px-3 py-2">最近一次探测</th>
            <th class="px-3 py-2" />
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
          <tr v-for="row in rows" :key="row.g.group_id" :class="row.g.enabled ? '' : 'opacity-60'">
            <td class="px-3 py-2"><Toggle v-model="row.g.enabled" /></td>
            <td class="truncate px-3 py-2" :title="`${row.platform} / ${row.name}`">
              <span class="text-gray-400">{{ row.platform }} /</span> {{ row.name }}
            </td>
            <td class="px-3 py-2">
              <select v-model.number="row.g.api_key_id" class="input w-full">
                <option :value="0">{{ keysFor(row.g.group_id).length ? '请选择' : '无绑定该分组的 Key' }}</option>
                <option v-for="k in keysFor(row.g.group_id)" :key="k.id" :value="k.id">{{ k.name }} · {{ k.masked }}</option>
              </select>
            </td>
            <td class="px-3 py-2">
              <select v-model="row.g.format" class="input w-full">
                <option value="">自动（{{ FORMAT_LABEL[autoFormat(row.platform)] }}）</option>
                <option v-for="(label, f) in FORMAT_LABEL" :key="f" :value="f">{{ label }}</option>
              </select>
            </td>
            <td class="px-3 py-2">
              <input v-model.trim="row.g.model" class="input w-full" placeholder="如 claude-haiku-4-5" />
            </td>
            <td class="px-3 py-2 text-xs">
              <template v-if="stateOf(row.g.group_id)?.last_checked_at">
                <span :class="stateOf(row.g.group_id)!.last_ok ? 'text-emerald-600' : 'text-red-500'">
                  {{ stateOf(row.g.group_id)!.last_ok ? '✓ 成功' : '✗ 失败' }}
                </span>
                <span v-if="stateOf(row.g.group_id)!.latency_ms" class="text-gray-400"> {{ (stateOf(row.g.group_id)!.latency_ms! / 1000).toFixed(1) }}s</span>
                <span class="text-gray-400"> · {{ ago(stateOf(row.g.group_id)!.last_checked_at!) }}</span>
                <div v-if="!stateOf(row.g.group_id)!.last_ok && stateOf(row.g.group_id)!.message" class="truncate text-gray-400" :title="stateOf(row.g.group_id)!.message">
                  {{ stateOf(row.g.group_id)!.message }}
                </div>
                <div class="text-gray-400">今日 {{ todayCount(row.g.group_id) }} / {{ p.daily_limit }} 次</div>
              </template>
              <span v-else class="text-gray-400">—</span>
            </td>
            <td class="px-3 py-2 text-right">
              <button
                type="button"
                class="btn btn-secondary btn-sm whitespace-nowrap"
                :disabled="running.has(row.g.group_id)"
                @click="probeNow(row.g)"
              >{{ running.has(row.g.group_id) ? '探测中…' : '立即探测' }}</button>
            </td>
          </tr>
        </tbody>
      </table>
      <p class="px-4 py-3 text-xs text-gray-400">
        Key 只能选当前管理员自己名下、已绑定该分组的可用 Key（与上游 V1「使用我的 Key」一致），探测会产生少量计费。
        「立即探测」使用页面上未保存的设置，跳过冷却和每日上限，结果会计入当前状态。
      </p>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { listProbeKeys, listProbeStates, runProbe, type GroupProbe, type ProbeFormat, type ProbeKey, type ProbeState } from '../api'
import { settings } from './state'
import NumField from './NumField.vue'

type ConcreteFormat = Exclude<ProbeFormat, ''>

const FORMAT_LABEL: Record<ConcreteFormat, string> = {
  anthropic: 'Anthropic Messages',
  openai_responses: 'OpenAI Responses',
  openai_chat: 'OpenAI Chat',
  gemini: 'Gemini'
}

// 与后端 ResolveFormat 保持一致
function autoFormat(platform: string): ConcreteFormat {
  switch (platform) {
    case 'anthropic':
    case 'antigravity':
    case 'kiro':
      return 'anthropic'
    case 'openai':
      return 'openai_responses'
    case 'gemini':
      return 'gemini'
    default:
      return 'openai_chat'
  }
}

const appStore = useAppStore()
const origin = window.location.origin
const p = computed(() => settings.draft?.probe)
const keys = ref<ProbeKey[]>([])
const states = ref<Record<number, ProbeState>>({})
const running = reactive(new Set<number>())

// 行顺序跟随「展示编排」，编排里没有的分组排在最后
const rows = computed(() => {
  const draft = settings.draft
  if (!draft) return []
  const order = new Map<number, number>()
  draft.layout.forEach((pl) => pl.groups.forEach((g) => order.set(g.group_id, order.size)))
  return draft.probe.groups
    .filter((g) => settings.groups[g.group_id])
    .map((g) => {
      const info = settings.groups[g.group_id]
      return { g, name: info.name, platform: info.platform, idx: order.get(g.group_id) ?? Number.MAX_SAFE_INTEGER }
    })
    .sort((a, b) => a.idx - b.idx)
})

function keysFor(groupId: number) {
  return keys.value.filter((k) => k.group_id === groupId)
}

function stateOf(groupId: number): ProbeState | undefined {
  return states.value[groupId]
}

function todayCount(groupId: number): number {
  const st = states.value[groupId]
  if (!st) return 0
  const today = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai' }).format(new Date())
  return st.day === today ? st.day_count : 0
}

function ago(iso: string): string {
  const sec = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000))
  if (sec < 60) return `${sec} 秒前`
  if (sec < 3600) return `${Math.floor(sec / 60)} 分钟前`
  if (sec < 86400) return `${Math.floor(sec / 3600)} 小时前`
  return `${Math.floor(sec / 86400)} 天前`
}

async function refreshStates() {
  const list = await listProbeStates()
  states.value = Object.fromEntries(list.map((s) => [s.group_id, s]))
}

async function probeNow(g: GroupProbe) {
  if (!p.value) return
  running.add(g.group_id)
  try {
    const res = await runProbe(p.value, g)
    if (res.ok) appStore.showSuccess(`探测成功，耗时 ${(res.latency_ms / 1000).toFixed(1)}s`)
    else appStore.showError(`探测失败：${res.message || '未知错误'}`)
    await refreshStates()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '探测失败'))
  } finally {
    running.delete(g.group_id)
  }
}

onMounted(async () => {
  try {
    const [k] = await Promise.all([listProbeKeys(), refreshStates()])
    keys.value = k
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '加载探测信息失败'))
  }
})
</script>
