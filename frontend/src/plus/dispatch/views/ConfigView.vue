<!-- [plus] 上游调度台 · 检测配置：定时检测开关；检测项（计划、覆盖、没覆盖的模型、近 24 小时、花费，都能编辑 / 停用 / 删除）；
     检测怎么跑（范围规则写在页面上）；健康分规则（默认和渠道状态同一套，改了马上看到正常 / 降级 / 异常各几个） -->
<template>
  <AppLayout>
    <div class="space-y-4" data-testid="dispatch-config">
      <div v-if="!cfg" class="py-16 text-center text-sm text-gray-400">{{ error || '加载中…' }}</div>
      <template v-else>
        <div class="card divide-y divide-gray-100 dark:divide-dark-700">
          <Row label="定时检测"><Toggle :model-value="cfg.config.detect" @update:model-value="saveCfg({ detect: $event })" /></Row>
        </div>

        <div class="flex items-center px-1 pt-2">
          <span class="text-sm text-gray-500">检测项</span>
          <button type="button" class="btn btn-primary btn-sm ml-auto" data-testid="dispatch-item-new" @click="edit(null)">新建检测项</button>
        </div>
        <div class="card overflow-x-auto p-0">
          <table class="table min-w-[1180px]">
            <thead><tr><th>检测项</th><th>计划</th><th>覆盖</th><th>没覆盖的模型</th><th>近 24 小时</th><th>未通过</th><th>花费</th><th>启用</th><th></th></tr></thead>
            <tbody>
              <tr v-for="it in items" :key="it.id" data-testid="dispatch-item-row">
                <td class="whitespace-nowrap"><div class="font-semibold">{{ it.name }}</div><div class="text-xs text-gray-500">{{ it.spec.name }}</div></td>
                <td class="min-w-[320px]">
                  <div v-if="!it.plans.length" class="text-sm text-gray-400">还没有计划</div>
                  <div v-for="p in it.plans" :key="p.id" class="flex items-center gap-2 py-0.5 text-xs" :class="p.enabled ? '' : 'opacity-50'">
                    <Toggle :model-value="p.enabled" @update:model-value="togglePlan(p.id, $event)" />
                    <span class="min-w-0 flex-1 truncate" :title="planTitle(p)">{{ platformName(p.platform) }} · <span class="font-mono">{{ someNames(p.models) }}</span> · {{ p.account_ids.length ? `${p.account_ids.length} 个账号` : '全部账号' }} · {{ everyText(p.interval_minutes) }}</span>
                    <span class="whitespace-nowrap text-gray-400">测 {{ p.targets }} 个</span>
                    <button type="button" class="whitespace-nowrap text-primary-600 hover:underline" @click="runPlan(p.id)">立即执行</button>
                  </div>
                </td>
                <td class="whitespace-nowrap"><div class="font-semibold tabular-nums">{{ it.covered == null ? '—' : `${it.covered}%` }}<small class="ml-1 text-xs font-normal text-gray-400">的流量</small></div><div class="text-xs text-gray-500">{{ it.targets }} 个账号 × 模型</div></td>
                <td class="whitespace-nowrap">
                  <template v-if="it.missing.length">
                    <div class="font-mono text-sm font-semibold" :title="it.missing.map((m) => `${missName(m.model)}  占流量 ${m.share}%`).join('\n')">{{ missName(it.missing[0].model) }}</div>
                    <div class="text-xs text-gray-500">{{ it.missing.length > 1 ? `等 ${it.missing.length} 个模型 · ` : '' }}占流量 {{ Math.round(it.missing.reduce((s, m) => s + m.share, 0)) }}%</div>
                  </template>
                  <span v-else class="text-gray-400">—</span>
                </td>
                <td class="whitespace-nowrap"><div class="font-semibold tabular-nums">{{ it.runs24 ? `${num(it.runs24)} 次` : '—' }}</div><div class="text-xs text-gray-500">{{ it.spec.judges ? (it.judged24 ? `通过 ${Math.round((it.pass24 * 100) / it.judged24)}%` : '\u00a0') : '只存作品，不判对错' }}</div></td>
                <td class="whitespace-nowrap"><span v-if="it.spec.judges" class="font-semibold tabular-nums" :class="it.failing ? 'text-red-600' : ''">{{ it.failing }} 个</span><span v-else class="text-gray-400">—</span></td>
                <td class="whitespace-nowrap"><div class="font-semibold tabular-nums">{{ it.tokens24 ? tok(it.tokens24) : '—' }}<small v-if="it.tokens24" class="ml-1 text-xs font-normal text-gray-400">token</small></div><div v-if="it.runs24" class="text-xs text-gray-500">平均每次 {{ tok(it.tokens24 / it.runs24) }}</div></td>
                <td><Toggle :model-value="it.enabled" @update:model-value="toggleItem(it, $event)" /></td>
                <td class="whitespace-nowrap text-right">
                  <button type="button" class="btn btn-secondary btn-sm" @click="edit(it)">编辑</button>
                  <button type="button" class="btn btn-ghost btn-sm text-red-600" @click="removing = it">删除</button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="px-1 pt-2 text-sm text-gray-500">检测怎么跑</div>
        <div class="card divide-y divide-gray-100 dark:divide-dark-700">
          <Row label="检测哪些账号"><span class="text-gray-900 dark:text-white">调度开着的 {{ cfg.serving }} 个</span><span class="ml-3">调度已关闭的 {{ cfg.closed }} 个不测</span></Row>
          <Row label="检测哪些模型"><span class="text-gray-900 dark:text-white">计划里选的、账号自己提供的模型</span></Row>
          <Row label="暂停或调低权重的账号"><span class="text-gray-900 dark:text-white">照常检测</span></Row>
          <Row label="没有流量的备用号">每 <NumInput :value="cfg.config.idle_minutes" @change="saveCfg({ idle_minutes: $event })" /> 分钟测一次（不低于计划频率）</Row>
          <Row label="同时检测"><NumInput :value="cfg.config.concurrency" @change="saveCfg({ concurrency: $event })" /> 个</Row>
          <Row label="连续出错">检测请求连着 <NumInput :value="cfg.config.fault_runs" @change="saveCfg({ fault_runs: $event })" /> 次出错算「测不了」</Row>
        </div>

        <div class="flex flex-wrap items-center gap-3 px-1 pt-2 text-sm text-gray-500">
          健康分
          <span class="ml-auto inline-flex items-center gap-3 text-xs">
            <span v-for="s in STATUS_ORDER" :key="s" class="inline-flex items-center gap-1.5"><i class="h-2 w-2 rounded-full" :class="STATUS_META[s].dot" />{{ STATUS_META[s].label }} {{ cfg.counts[s] ?? 0 }}</span>
          </span>
        </div>
        <div class="card divide-y divide-gray-100 dark:divide-dark-700">
          <Row label="和渠道状态用同一套规则"><Toggle :model-value="cfg.config.same_as_channel" @update:model-value="saveCfg({ same_as_channel: $event, rules: $event ? cfg.config.rules : { ...cfg.effective_rules } })" /></Row>
          <Row label="正常">健康分不低于 <NumInput :value="R.up_min" :disabled="same" @change="setRule('up_min', $event)" /></Row>
          <Row label="降级">健康分不低于 <NumInput :value="R.warn_min" :disabled="same" @change="setRule('warn_min', $event)" />，再低是异常</Row>
          <Row label="失败率">到 <NumInput :value="Math.round(R.critical_error_rate * 100)" :disabled="same" @change="setRule('critical_error_rate', $event / 100)" /> % 时这一项记 0 分</Row>
          <Row label="首字耗时"><NumInput :value="R.target_ttft_ms / 1000" :disabled="same" @change="setRule('target_ttft_ms', $event * 1000)" /> 秒以内满分，<NumInput :value="R.critical_ttft_ms / 1000" :disabled="same" @change="setRule('critical_ttft_ms', $event * 1000)" /> 秒以上记 0 分</Row>
          <Row label="比重">失败率 <NumInput :value="Math.round(R.error_weight * 100)" :disabled="same" @change="setRule('error_weight', $event / 100)" />，首字耗时 <NumInput :value="Math.round(R.ttft_weight * 100)" :disabled="same" @change="setRule('ttft_weight', $event / 100)" />，缓存 <NumInput :value="Math.round(R.cache_weight * 100)" :disabled="same" @change="setRule('cache_weight', $event / 100)" /></Row>
          <Row label="最少请求数">状态条每一格 <NumInput :value="R.history_min_sample" :disabled="same" @change="setRule('history_min_sample', $event)" /> 次，当前状态 <NumInput :value="R.min_sample" :disabled="same" @change="setRule('min_sample', $event)" /> 次</Row>
        </div>
      </template>
    </div>
    <ItemDialog :show="editing" :source="source" :meta="meta" @close="editing = false" @saved="load" />
    <ConfirmDialog
      :show="!!removing" title="删除检测项" danger confirm-text="删除"
      :message="`删除「${removing?.name}」后，它的计划和检测记录都会一起删除，不能恢复。`" @confirm="remove" @cancel="removing = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref } from 'vue'
import { useRoute } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { STATUS_ORDER } from '@/plus/channelstatus/status'
import { dispatchApi, type Config, type ConfigView, type HealthRules, type ItemView, type Meta, type Plan } from '../api'
import ItemDialog from '../shared/ItemDialog.vue'
import { STATUS_META, everyText, num, platformName, someNames, tok } from '../shared/format'

// 设置行：左边粗体标签，右边内容
const Row = defineComponent({
  props: { label: { type: String, required: true } },
  setup: (p, { slots }) => () =>
    h('div', { class: 'flex min-h-[52px] items-center gap-2 px-5 py-2.5 text-sm text-gray-500' }, [
      h('span', { class: 'text-[15px] font-semibold text-gray-900 dark:text-white' }, p.label),
      h('span', { class: 'flex-1' }),
      slots.default?.()
    ])
})
// 数字框：失焦或回车时提交
const NumInput = defineComponent({
  props: { value: { type: Number, required: true }, disabled: Boolean },
  emits: ['change'],
  setup: (p, { emit }) => () =>
    h('input', {
      type: 'number', value: p.value, disabled: p.disabled,
      class: 'input inline-block w-20 px-2 py-1 text-center font-semibold disabled:bg-transparent disabled:shadow-none',
      onChange: (e: Event) => emit('change', Number((e.target as HTMLInputElement).value))
    })
})

const route = useRoute()
const appStore = useAppStore()
const cfg = ref<ConfigView | null>(null)
const items = ref<ItemView[]>([])
const meta = ref<Meta | null>(null)
const error = ref('')
const editing = ref(false)
const source = ref<ItemView | null>(null)
const removing = ref<ItemView | null>(null)

const same = computed(() => !!cfg.value?.config.same_as_channel)
const R = computed<HealthRules>(() => (same.value ? cfg.value!.effective_rules : cfg.value!.config.rules))

async function load() {
  try {
    const [c, i, m] = await Promise.all([dispatchApi.config(), dispatchApi.items(), dispatchApi.meta()])
    cfg.value = c
    items.value = i
    meta.value = m
    error.value = ''
  } catch (e) {
    error.value = extractApiErrorMessage(e)
  }
}
void load().then(() => {
  const want = Number(route.query.item)
  const it = items.value.find((x) => x.id === want)
  if (it) edit(it)
})

async function act(fn: () => Promise<unknown>, ok: string) {
  try {
    await fn()
    appStore.showSuccess(ok)
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e))
  }
  await load()
}

function saveCfg(patch: Partial<Config>) {
  const c = cfg.value!
  return act(async () => {
    cfg.value = await dispatchApi.saveConfig({ ...c.config, ...patch }, c.version)
  }, '已保存')
}
function setRule(k: keyof HealthRules, v: number) {
  void saveCfg({ rules: { ...cfg.value!.config.rules, [k]: v } })
}

const missName = (k: string) => {
  const [p, m] = k.split('|')
  return `${platformName(p)} ${m}`
}
const planTitle = (p: Plan) => {
  const names = (meta.value?.accounts[p.platform] ?? []).filter((a) => p.account_ids.includes(a.id)).map((a) => a.name)
  return `${p.models.join('、')}\n${names.length ? names.join('、') : '全部账号'}`
}

function edit(it: ItemView | null) {
  source.value = it
  editing.value = true
}
const toggleItem = (it: ItemView, on: boolean) => act(() => dispatchApi.toggleItem(it.id, on), on ? '已启用' : '已停用')
const togglePlan = (id: number, on: boolean) => act(() => dispatchApi.togglePlan(id, on), on ? '计划已启用' : '计划已停用')
async function runPlan(id: number) {
  try {
    const r = await dispatchApi.runPlan(id)
    appStore.showSuccess(r.queued ? `已排队 ${r.queued} 次检测` : '没有要测的，或者都已经在排队了')
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e))
  }
}
function remove() {
  const it = removing.value!
  removing.value = null
  void act(() => dispatchApi.deleteItem(it.id), '已删除')
}
</script>
