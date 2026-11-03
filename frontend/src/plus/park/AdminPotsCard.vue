<template>
  <div v-if="cfg && st" class="card space-y-5 p-5">
    <div>
      <p class="font-medium text-gray-900 dark:text-white">奖金罐</p>
      <p class="text-xs text-gray-500 dark:text-gray-400">
        用户每摇一次，消耗的积分按比例放进两个罐，剩下的回收销毁。所有奖品都从罐里出，罐里不够付的奖品这一次抽不到（概率转给谢谢惠顾）。
      </p>
    </div>

    <!-- 注入比例 -->
    <div class="flex flex-wrap items-center gap-5 text-sm text-gray-600 dark:text-gray-400">
      <label class="flex items-center gap-2">小奖罐 <input v-model.number="cfg.pot.small_pct" type="number" min="0" max="100" class="input w-20" />%</label>
      <label class="flex items-center gap-2">大奖罐 <input v-model.number="cfg.pot.jackpot_pct" type="number" min="0" max="100" class="input w-20" />%</label>
      <span :class="burnPct < 0 ? 'text-rose-500' : ''">回收 {{ burnPct }}%</span>
      <span class="text-xs text-gray-400">用户长期最多拿回 {{ 100 - Math.max(0, burnPct) }}%</span>
    </div>

    <!-- 每摇一次：注入 vs 期望支出 -->
    <table class="w-full text-sm">
      <thead class="text-left text-xs text-gray-400">
        <tr>
          <th class="pb-2">每摇一次</th>
          <th class="pb-2">注入</th>
          <th class="pb-2">期望支出（无加成）</th>
          <th class="pb-2">期望支出（连签加成全开）</th>
        </tr>
      </thead>
      <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
        <tr v-for="row in preview" :key="row.pot">
          <td class="py-2 text-gray-800 dark:text-gray-200">{{ row.name }}</td>
          <td class="py-2 tabular-nums">{{ row.inflow }}</td>
          <td class="py-2 tabular-nums" :class="row.base > row.inflow ? 'text-rose-500' : ''">{{ row.base.toFixed(1) }}</td>
          <td class="py-2 tabular-nums" :class="row.boosted > row.inflow ? 'text-rose-500' : ''">{{ row.boosted.toFixed(1) }}</td>
        </tr>
      </tbody>
    </table>
    <p class="text-xs text-gray-500 dark:text-gray-400">
      期望支出大于注入（标红）时，罐子会越来越空，奖品会经常被排除。大奖罐每摇一次进 {{ preview[1].inflow }} 积分，攒够最贵的大奖（{{ jackpotMax }} 积分）约需 {{ jackpotSpins }} 次。
    </p>

    <!-- 罐子当前状态 -->
    <div class="grid gap-4 md:grid-cols-2">
      <div v-for="p in st.pots" :key="p.pot" class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
        <div class="flex items-baseline justify-between">
          <span class="font-medium text-gray-800 dark:text-gray-200">{{ potName(p.pot) }}</span>
          <span class="text-2xl font-semibold tabular-nums text-gray-900 dark:text-white" :data-testid="`pot-balance-${p.pot}`">{{ p.balance }}</span>
        </div>
        <dl class="mt-2 grid grid-cols-2 gap-x-4 gap-y-1 text-xs text-gray-500">
          <dt>今日注入 / 支出</dt>
          <dd class="text-right tabular-nums">{{ p.today_inflow }} / {{ p.today_payout }}</dd>
          <dt>今日人工调整</dt>
          <dd class="text-right tabular-nums">{{ p.today_manual }}</dd>
          <dt>累计注入 / 支出</dt>
          <dd class="text-right tabular-nums">{{ p.total_inflow }} / {{ p.total_payout }}</dd>
        </dl>

        <!-- 奖品解锁进度：罐里余额 ≥ 奖品价值时这一档才进入抽奖 -->
        <div class="mt-3 border-t border-gray-100 pt-3 dark:border-dark-700" :data-testid="`pot-unlock-${p.pot}`">
          <div class="mb-1.5 flex items-baseline justify-between text-xs">
            <span class="font-medium text-gray-700 dark:text-gray-300">奖品解锁进度</span>
            <span class="text-gray-400">每摇一次净增约 {{ fmtNet(netPerSpin(p.pot as PotKind, p.balance)) }} 积分</span>
          </div>
          <p v-if="!unlockRows(p.pot as PotKind, p.balance).length" class="text-xs text-gray-400">没有从这个罐出奖的奖品</p>
          <div v-for="r in unlockRows(p.pot as PotKind, p.balance)" :key="r.idx" class="py-1 text-xs">
            <div class="flex items-center justify-between gap-2">
              <span class="truncate text-gray-700 dark:text-gray-300">{{ r.name }} <span class="text-gray-400">· 价值 {{ r.value }}</span></span>
              <span v-if="r.ready" class="shrink-0 font-medium text-emerald-600">可抽中</span>
              <span v-else class="shrink-0 text-amber-600">还差 {{ r.gap }}{{ r.eta ? ` · ${r.eta}` : '' }}</span>
            </div>
            <div class="mt-1 h-1.5 overflow-hidden rounded bg-gray-100 dark:bg-dark-700">
              <div class="h-full rounded" :class="r.ready ? 'bg-emerald-500' : 'bg-amber-400'" :style="{ width: `${r.pct}%` }" />
            </div>
          </div>
        </div>
      </div>
    </div>
    <p class="text-xs text-gray-500 dark:text-gray-400">
      奖金罐没有容量上限，也不会"满了才出奖"：只要罐里余额够付某个奖品，这个奖品就按配置概率参与抽奖，抽中后从罐里扣。
      净增 = 每次注入 − 当前可抽中奖品的期望支出；预计时间按近 7 天日均 {{ dailySpins.toFixed(1) }} 次摇奖估算，只是参考。
    </p>
    <p class="text-sm text-gray-600 dark:text-gray-400">
      累计摇奖 {{ st.spins.total_spins }} 次（今日 {{ st.spins.today_spins }} 次），实际返奖率
      <span class="font-semibold tabular-nums">{{ actualReturn }}</span>，目标 {{ 100 - Math.max(0, burnPct) }}%
    </p>

    <!-- 加水 / 抽水 -->
    <div class="flex flex-wrap items-end gap-3 rounded-lg bg-gray-50 p-4 dark:bg-dark-800">
      <label class="text-sm">
        <span class="block text-xs text-gray-500">奖金罐</span>
        <select v-model="adj.pot" class="input mt-1 w-28">
          <option value="small">小奖罐</option>
          <option value="jackpot">大奖罐</option>
        </select>
      </label>
      <label class="text-sm">
        <span class="block text-xs text-gray-500">操作</span>
        <select v-model="adj.dir" class="input mt-1 w-24">
          <option value="add">加水</option>
          <option value="sub">抽水</option>
        </select>
      </label>
      <label class="text-sm">
        <span class="block text-xs text-gray-500">积分</span>
        <input v-model.number="adj.amount" type="number" min="1" class="input mt-1 w-28" />
      </label>
      <label class="min-w-[200px] flex-1 text-sm">
        <span class="block text-xs text-gray-500">说明（必填，会记入流水）</span>
        <input v-model="adj.note" maxlength="200" class="input mt-1 w-full" placeholder="如：上线初始注入" />
      </label>
      <button type="button" class="btn btn-primary" :disabled="adjusting || !adj.amount || !adj.note.trim()" @click="doAdjust">
        {{ adjusting ? '提交中…' : '确认' }}
      </button>
    </div>

    <!-- 奖金罐流水 -->
    <div>
      <div class="mb-2 flex items-center justify-between">
        <p class="text-sm font-medium text-gray-800 dark:text-gray-200">奖金罐流水</p>
        <label class="flex items-center gap-2 text-xs text-gray-500">
          <input v-model="manualOnly" type="checkbox" class="accent-primary-500" @change="loadLedger" />只看人工调整
        </label>
      </div>
      <div class="max-h-72 overflow-y-auto">
        <table class="w-full text-xs">
          <thead class="sticky top-0 bg-white text-left text-gray-400 dark:bg-dark-900">
            <tr>
              <th class="py-1.5">时间</th>
              <th class="py-1.5">罐</th>
              <th class="py-1.5">类型</th>
              <th class="py-1.5 text-right">变动</th>
              <th class="py-1.5 text-right">余额</th>
              <th class="py-1.5 pl-3">说明</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="it in ledger" :key="it.id">
              <td class="py-1.5 text-gray-500">{{ fmtTime(it.created_at) }}</td>
              <td class="py-1.5">{{ potName(it.pot) }}</td>
              <td class="py-1.5">{{ REASONS[it.reason] ?? it.reason }}</td>
              <td class="py-1.5 text-right tabular-nums" :class="it.delta >= 0 ? 'text-emerald-600' : 'text-rose-500'">{{ it.delta >= 0 ? '+' : '' }}{{ it.delta }}</td>
              <td class="py-1.5 text-right tabular-nums">{{ it.balance }}</td>
              <td class="py-1.5 pl-3 text-gray-500">{{ it.note || (it.spin_id ? `抽奖 #${it.spin_id}` : '') }}</td>
            </tr>
            <tr v-if="!ledger.length">
              <td colspan="6" class="py-6 text-center text-gray-400">暂无流水</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAdjustPot, adminPotLedger, adminSlotStats, tierValuePoints, type PotKind, type PotLedgerItem } from '../slot/api'
import { FULL_BP, park, refreshSlotStats } from './adminState'

const appStore = useAppStore()

const st = computed(() => park.slot)
const cfg = computed(() => park.slot?.config)

const REASONS: Record<string, string> = { inflow: '摇奖注入', payout: '中奖支出', admin_add: '人工加水', admin_sub: '人工抽水' }
const potName = (p: string) => (p === 'jackpot' ? '大奖罐' : '小奖罐')

const burnPct = computed(() => 100 - (cfg.value?.pot.small_pct || 0) - (cfg.value?.pot.jackpot_pct || 0))

// 连签加成全开时的概率：加成部分从谢谢惠顾里扣，不够扣时按比例缩小（与后端一致）
function boostedProbs(): number[] {
  const tiers = cfg.value!.tiers
  const probs = tiers.map((t) => t.prob_bp || 0)
  const sb = park.checkin?.streak_boost
  if (!sb?.enabled) return probs
  const noneIdx = tiers.findIndex((t) => t.prize_type === 'none')
  if (noneIdx < 0) return probs
  const want = sb.tiers.reduce((s, b) => s + (b.add_bp || 0), 0)
  const scale = want > probs[noneIdx] && want > 0 ? probs[noneIdx] / want : 1
  let used = 0
  sb.tiers.forEach((b) => {
    if (b.tier === noneIdx || b.tier >= probs.length) return
    const add = Math.floor((b.add_bp || 0) * scale)
    probs[b.tier] += add
    used += add
  })
  probs[noneIdx] -= used
  return probs
}

function expect(probs: number[], pot: PotKind): number {
  const c = cfg.value!
  return c.tiers.reduce((s, t, i) => (t.pot === pot ? s + (probs[i] / FULL_BP) * tierValuePoints(t, c.points_per_yuan) : s), 0)
}

const preview = computed(() => {
  const c = cfg.value!
  const base = c.tiers.map((t) => t.prob_bp || 0)
  const boosted = boostedProbs()
  return (['small', 'jackpot'] as PotKind[]).map((pot) => ({
    pot,
    name: potName(pot),
    inflow: Math.floor(((c.cost_per_spin || 0) * (pot === 'small' ? c.pot.small_pct : c.pot.jackpot_pct)) / 100),
    base: expect(base, pot),
    boosted: expect(boosted, pot)
  }))
})

const jackpotMax = computed(() => {
  const c = cfg.value!
  return c.tiers.filter((t) => t.pot === 'jackpot').reduce((m, t) => Math.max(m, tierValuePoints(t, c.points_per_yuan)), 0)
})
const jackpotSpins = computed(() => (preview.value[1].inflow > 0 ? Math.ceil(jackpotMax.value / preview.value[1].inflow) : '∞'))

// ─── 奖品解锁进度 ──────────────────────────────────────────────────
// 近 7 天日均摇奖次数，用来把"还要摇几次"换算成大概的天数
const dailySpins = ref(0)
async function loadDailySpins() {
  try {
    const s = await adminSlotStats(7)
    dailySpins.value = s.range.spins / Math.max(1, s.days)
  } catch {
    dailySpins.value = 0
  }
}

/** 每摇一次该罐的净增：注入 − 当前余额下可抽中奖品的期望支出（无加成） */
function netPerSpin(pot: PotKind, balance: number): number {
  const c = cfg.value!
  const inflow = Math.floor(((c.cost_per_spin || 0) * (pot === 'small' ? c.pot.small_pct : c.pot.jackpot_pct)) / 100)
  const payout = c.tiers.reduce((s, t) => {
    const v = tierValuePoints(t, c.points_per_yuan)
    return t.pot === pot && v <= balance ? s + ((t.prob_bp || 0) / FULL_BP) * v : s
  }, 0)
  return inflow - payout
}
const fmtNet = (n: number) => (n >= 0 ? `+${n.toFixed(1)}` : n.toFixed(1))

function eta(gap: number, net: number): string {
  if (net <= 0) return '按当前配置攒不到，需加水或调配置'
  const spins = Math.ceil(gap / net)
  if (dailySpins.value <= 0) return `约 ${spins} 次摇奖`
  const days = spins / dailySpins.value
  return `约 ${spins} 次摇奖（${days < 1 ? '1 天内' : `约 ${Math.ceil(days)} 天`}）`
}

function unlockRows(pot: PotKind, balance: number) {
  const c = cfg.value!
  const net = netPerSpin(pot, balance)
  return c.tiers
    .map((t, idx) => ({ t, idx, value: tierValuePoints(t, c.points_per_yuan) }))
    .filter((x) => x.t.pot === pot && x.t.prize_type !== 'none' && x.value > 0)
    .sort((a, b) => a.value - b.value)
    .map(({ t, idx, value }) => {
      const gap = Math.max(0, value - balance)
      return {
        idx,
        name: `${t.emoji ?? ''} ${t.label}`.trim(),
        value,
        ready: gap === 0,
        gap,
        pct: Math.min(100, Math.round((balance / value) * 100)),
        eta: gap ? eta(gap, net) : ''
      }
    })
}

const actualReturn = computed(() => {
  const s = st.value!
  if (!s.spins.total_cost) return '—'
  const payout = s.pots.reduce((sum, p) => sum + p.total_payout, 0)
  return `${((payout / s.spins.total_cost) * 100).toFixed(1)}%`
})

// ─── 加水 / 抽水 ────────────────────────────────────────────────────
const adj = reactive({ pot: 'small' as PotKind, dir: 'add' as 'add' | 'sub', amount: 0, note: '' })
const adjusting = ref(false)

async function doAdjust() {
  const delta = adj.dir === 'add' ? adj.amount : -adj.amount
  if (!window.confirm(`确认${adj.dir === 'add' ? '向' : '从'}${potName(adj.pot)}${adj.dir === 'add' ? '加入' : '抽出'} ${adj.amount} 积分？`)) return
  adjusting.value = true
  try {
    const bal = await adminAdjustPot(adj.pot, delta, adj.note.trim())
    appStore.showSuccess(`${potName(adj.pot)}当前余额 ${bal}`)
    adj.amount = 0
    adj.note = ''
    await Promise.all([refreshSlotStats(), loadLedger()])
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '调整失败'))
  } finally {
    adjusting.value = false
  }
}

// ─── 流水 ──────────────────────────────────────────────────────────
const manualOnly = ref(false)
const ledger = ref<PotLedgerItem[]>([])

async function loadLedger() {
  try {
    ledger.value = await adminPotLedger(manualOnly.value, 100)
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '流水加载失败'))
  }
}

function fmtTime(iso: string): string {
  const d = new Date(iso)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

onMounted(() => {
  void loadLedger()
  void loadDailySpins()
})
</script>
