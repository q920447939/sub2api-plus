<!-- [plus] 一条调整记录：一行摘要，展开是调整前 → 调整后的对比
     （健康分、各账号的占比、每一项指标变好变差、调整后全部失败的模型、受影响分组的状态条、按模型细分） -->
<template>
  <div class="border-t border-gray-100 first:border-t-0 dark:border-dark-700" data-testid="dispatch-record">
    <div class="grid cursor-pointer grid-cols-[96px_minmax(150px,1.2fr)_130px_minmax(0,1.4fr)_120px_80px_14px] items-center gap-4 px-5 py-3.5 hover:bg-gray-50 dark:hover:bg-dark-800" @click="open = !open">
      <span class="text-xs tabular-nums text-gray-500">{{ dt(rec.created_at) }}</span>
      <div class="min-w-0">
        <div class="truncate font-semibold">{{ name(rec.account_id) }}</div>
        <div class="truncate text-xs text-gray-500"><span v-if="rec.model" class="font-mono">{{ rec.model }} · </span>{{ someNames(rec.groups.map((g) => gname(g.group_id))) }}</div>
      </div>
      <span class="text-sm">
        <template v-if="same">{{ ruleText(rec.groups[0]?.from) }} <span class="text-gray-400">→</span> <b>{{ ruleText(rec.groups[0]?.to) }}</b></template>
        <b v-else>{{ rec.groups.length }} 个分组分别调整</b>
      </span>
      <span class="truncate text-xs text-gray-500">{{ rec.note }}<template v-if="rec.operator"> · {{ rec.operator.split('@')[0] }}</template></span>
      <span v-if="e.fresh" class="text-sm text-gray-400">等待数据</span>
      <span v-else class="text-sm tabular-nums">{{ e.score_before ?? '--' }} <span class="text-gray-400">→</span> <b :class="LV.score(e.score_after)">{{ e.score_after ?? '--' }}</b></span>
      <span class="text-xs font-semibold">
        <template v-if="!e.fresh && e.enough"><span v-if="e.worse" class="text-red-600">▼ {{ e.worse }}</span> <span v-if="e.better" class="text-emerald-600">▲ {{ e.better }}</span></template>
      </span>
      <span class="text-gray-400 transition-transform" :class="open ? 'rotate-90' : ''">›</span>
    </div>

    <div v-if="open" class="border-t border-gray-100 bg-gray-50/60 p-5 dark:border-dark-700 dark:bg-dark-800/40">
      <div class="grid gap-9 lg:grid-cols-[320px_minmax(0,1fr)]">
        <div>
          <div class="text-xs text-gray-500">健康分</div>
          <div class="text-3xl font-bold tabular-nums"><span :class="LV.score(e.score_before)">{{ e.score_before ?? '--' }}</span> <span class="text-xl text-gray-400">→</span> <span :class="e.fresh ? 'text-gray-400' : LV.score(e.score_after)">{{ e.fresh ? '—' : e.score_after ?? '--' }}</span></div>
          <template v-if="e.fresh">
            <div class="mb-1 mt-4 text-xs text-gray-500">预计承接</div>
            <div v-for="p in rec.predicted" :key="p.group_id" class="flex py-0.5 text-sm"><span class="flex-1">{{ gname(p.group_id) }}</span><span class="tabular-nums text-gray-500">{{ pct(p.now) }} → <b class="text-gray-900 dark:text-white">{{ pct(p.next) }}</b></span></div>
          </template>
          <template v-else>
            <div class="my-3 grid grid-cols-[44px_1fr] items-center gap-x-2.5 gap-y-1.5 text-xs text-gray-500">
              <span>调整前</span><Seg :parts="e.shares.map((s) => ({ id: s.account_id, v: s.before }))" :color="color" :name="name" />
              <span>调整后</span><Seg :parts="e.shares.map((s) => ({ id: s.account_id, v: s.after ?? 0 }))" :color="color" :name="name" />
            </div>
            <div v-for="s in e.shares" :key="s.account_id" class="flex items-center gap-2 py-0.5 text-sm">
              <i class="h-2.5 w-2.5 rounded-sm" :style="{ background: color(s.account_id) }" /><span class="flex-1 truncate">{{ name(s.account_id) }}</span>
              <span class="tabular-nums text-gray-500">{{ pct(s.before) }} → <b class="text-gray-900 dark:text-white">{{ pct(s.after) }}</b></span>
            </div>
          </template>
          <template v-if="rec.side.length">
            <div class="mb-1 mt-4 text-xs text-gray-500">同组连带调整</div>
            <div v-for="x in rec.side" :key="`${x.account_id}-${x.group_id}-${x.model}`" class="flex items-center gap-2 py-0.5 text-sm">
              <span class="flex-1 truncate">{{ name(x.account_id) }} <span class="text-xs text-gray-400">{{ gname(x.group_id) }}</span></span>
              <span class="tabular-nums text-gray-500">{{ ruleText(x.from) }} → <b class="text-gray-900 dark:text-white">{{ ruleText(x.to) }}</b></span>
            </div>
          </template>
        </div>
        <div>
          <div v-if="e.fresh" class="mb-1.5 text-xs text-gray-500">刚调整，等新请求进来</div>
          <div v-else-if="!e.enough" class="mb-1.5 text-xs text-gray-500">调整后只有 {{ e.na }} 次请求，先看个大概</div>
          <div v-for="x in shownItems" :key="x.key" class="flex items-center gap-2.5 border-b border-gray-100 py-2 text-sm last:border-b-0 dark:border-dark-700">
            <span v-if="!e.fresh" class="rounded-md px-1.5 text-xs font-semibold" :class="TREND[x.trend].cls">{{ TREND[x.trend].t }}</span>
            <span>{{ x.label }}</span>
            <span class="ml-auto tabular-nums text-gray-500">{{ unitVal(x.unit, x.before) }}<template v-if="!e.fresh"> → <b class="text-gray-900 dark:text-white">{{ unitVal(x.unit, x.after) }}</b></template></span>
          </div>
          <div v-if="e.enough && !moved.length" class="text-xs text-gray-500">各项都没有明显变化</div>
          <button v-if="e.enough && quiet.length" type="button" class="mt-2 text-xs text-primary-600" @click="flat = !flat">{{ flat ? '收起' : `其余 ${quiet.length} 项没有明显变化` }}</button>
        </div>
      </div>

      <div v-for="d in e.dead" :key="d.account_id" class="mt-4 flex flex-wrap items-center gap-3 rounded-xl bg-amber-50 px-4 py-2.5 text-sm text-amber-900 dark:bg-amber-900/20 dark:text-amber-200">
        <span>{{ name(d.account_id) }} 在 <b class="font-mono">{{ d.models.join('、') }}</b> 上 {{ d.n }} 次请求全部失败</span>
        <button type="button" class="btn btn-secondary btn-sm ml-auto" @click="emit('moveAway', { accountId: d.account_id, models: d.models })">把{{ d.models.length > 1 ? '这些' : '这个' }}模型从它身上挪走</button>
      </div>

      <div class="mb-1 mt-5 text-xs text-gray-400">受影响的分组</div>
      <div v-for="g in e.groups" :key="g.group_id" class="grid grid-cols-[minmax(150px,1.1fr)_minmax(220px,2.6fr)_110px] items-center gap-4 py-2">
        <span class="truncate font-semibold">{{ gname(g.group_id!) }}</span>
        <StatusBar :bars="g.bars ?? []" :marks="[rec.created_at]" />
        <span class="text-right text-sm tabular-nums text-gray-500">{{ g.sb ?? '--' }} → <b :class="LV.score(g.sa)">{{ e.fresh ? '—' : g.sa ?? '--' }}</b></span>
      </div>

      <div v-if="byModel && e.models.length" class="mt-3 overflow-x-auto">
        <table class="table text-xs">
          <thead><tr><th>模型</th><th>请求</th><th v-for="m in METRICS" :key="m">{{ e.models[0].cols[m]?.label }}</th></tr></thead>
          <tbody>
            <tr v-for="m in e.models" :key="m.model">
              <td class="font-mono">{{ m.model }}</td>
              <td class="tabular-nums">{{ m.nb }} → {{ m.na }}</td>
              <td v-for="k in METRICS" :key="k" class="whitespace-nowrap tabular-nums">
                {{ unitVal(m.cols[k].unit, m.cols[k].before) }} → <b>{{ e.fresh ? '—' : unitVal(m.cols[k].unit, m.cols[k].after) }}</b>
                <span v-if="m.cols[k].trend === 'worse'" class="ml-1 text-red-600">▼</span><span v-else-if="m.cols[k].trend === 'better'" class="ml-1 text-emerald-600">▲</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="mt-4 flex flex-wrap items-center gap-4 text-xs text-gray-400">
        <span>{{ e.fresh ? `调整前 ${e.hours} 小时 ${num(e.nb)} 次请求` : `前后各 ${e.hours} 小时 · ${num(e.nb)} → ${num(e.na)} 次请求` }}</span>
        <button v-if="e.models.length > 1" type="button" class="text-primary-600" @click="byModel = !byModel">{{ byModel ? '收起模型' : '按模型看' }}</button>
        <span class="flex-1" />
        <span v-if="rec.undone_at">已恢复原样</span>
        <span v-else-if="rec.kind === 'undo'">这是一次恢复</span>
        <button v-else type="button" class="btn btn-secondary btn-sm" @click="emit('undo', rec.id)">恢复原样</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { NameMap, RecordView } from '../api'
import { LV, PALETTE, dt, num, pct, ruleText, someNames, unitVal } from './format'
import Seg from './Seg.vue'
import StatusBar from './StatusBar.vue'

const props = defineProps<{ rec: RecordView; names: NameMap; startOpen?: boolean }>()
const emit = defineEmits<{ undo: [id: number]; moveAway: [p: { accountId: number; models: string[] }] }>()

const METRICS = ['err', 'r429', 'ttft', 'tps', 'cache']
const TREND = {
  worse: { t: '▼ 变差', cls: 'bg-red-50 text-red-600 dark:bg-red-900/30' },
  better: { t: '▲ 变好', cls: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30' },
  flat: { t: '没变', cls: 'bg-gray-100 font-normal text-gray-400 dark:bg-dark-700' },
  na: { t: '数据不够', cls: 'bg-gray-100 font-normal text-gray-400 dark:bg-dark-700' }
}

const open = ref(!!props.startOpen)
const flat = ref(false)
const byModel = ref(false)
const e = computed(() => props.rec.effect)
const name = (id: number) => props.names.accounts[String(id)] ?? `#${id}`
const gname = (id: number) => props.names.groups[String(id)] ?? `#${id}`
const color = (id: number) => PALETTE[Math.max(0, e.value.shares.findIndex((s) => s.account_id === id)) % PALETTE.length]
const same = computed(() => props.rec.groups.every((g) => ruleText(g.to) === ruleText(props.rec.groups[0].to) && ruleText(g.from) === ruleText(props.rec.groups[0].from)))
const moved = computed(() => e.value.items.filter((x) => x.trend === 'worse' || x.trend === 'better'))
const quiet = computed(() => e.value.items.filter((x) => x.trend === 'flat' || x.trend === 'na'))
const shownItems = computed(() => (e.value.fresh || !e.value.enough ? e.value.items : flat.value ? [...moved.value, ...quiet.value] : moved.value))
</script>
