<template>
  <section class="nb-box flex min-h-[420px] flex-col xl:max-h-[calc(100vh-6rem)]" data-testid="park-records">
    <!-- 标签页 -->
    <div class="nb-line grid grid-cols-2 border-b-[3px]">
      <button
        v-for="t in TABS"
        :key="t.key"
        type="button"
        class="nb-head nb-line py-3 text-sm first:border-r-[3px]"
        :class="tab === t.key ? 'bg-black text-[#ffe600]' : 'nb-hover'"
        :data-testid="`slot-records-tab-${t.key}`"
        @click="tab = t.key"
      >{{ t.label }}</button>
    </div>

    <!-- 积分流水 -->
    <template v-if="tab === 'ledger'">
      <div class="flex flex-wrap gap-1.5 p-3">
        <button
          v-for="r in REASONS"
          :key="r.key"
          type="button"
          class="nb-tag"
          :class="reason === r.key ? 'bg-black text-[#ffe600]' : 'nb-surface nb-hover'"
          @click="setReason(r.key)"
        >{{ r.label }}</button>
      </div>
      <div class="flex-1 overflow-y-auto px-3">
        <p v-if="ledgerLoading && !ledger.length" class="nb-muted py-10 text-center text-sm font-bold">加载中…</p>
        <p v-else-if="!ledger.length" class="nb-muted py-10 text-center text-sm font-bold">暂无积分变动</p>
        <ul v-else class="nb-line divide-y-2 divide-dashed" style="--tw-divide-opacity: 1">
          <li v-for="it in ledger" :key="it.id" class="flex items-center gap-3 py-2.5" data-testid="slot-ledger-item">
            <span class="nb-line nb-surface-2 flex size-8 shrink-0 items-center justify-center border-2 text-base">{{ reasonIcon(it.reason) }}</span>
            <div class="min-w-0 flex-1">
              <p class="truncate text-sm font-bold">{{ it.note || reasonLabel(it.reason) }}</p>
              <p class="nb-muted text-[11px] font-bold">{{ fmt(it.created_at) }} · 余 {{ it.balance }}</p>
            </div>
            <span
              class="nb-head shrink-0 text-base tabular-nums"
              :class="it.delta >= 0 ? 'text-[#ff2e93]' : 'nb-muted'"
            >{{ it.delta >= 0 ? '+' : '' }}{{ it.delta }}</span>
          </li>
        </ul>
      </div>
      <div class="nb-line space-y-1 border-t-2 border-dashed p-3">
        <NbPager :page="ledgerPage" :total="ledgerTotal" :page-size="PAGE_SIZE" :disabled="ledgerLoading" @change="loadLedger" />
        <p v-if="ledgerTotal" class="nb-faint text-center text-[11px] font-bold">共 {{ ledgerTotal }} 条</p>
      </div>
    </template>

    <!-- 奖品记录 -->
    <template v-else>
      <div class="flex items-center justify-between gap-2 p-3">
        <div class="flex gap-1.5">
          <button
            v-for="f in PRIZE_FILTERS"
            :key="f.key"
            type="button"
            class="nb-tag"
            :class="prizeFilter === f.key ? 'bg-black text-[#ffe600]' : 'nb-surface nb-hover'"
            @click="prizeFilter = f.key"
          >{{ f.label }}</button>
        </div>
        <span class="nb-faint text-[11px] font-bold">共 {{ historyTotal }} 条</span>
      </div>
      <div class="flex-1 space-y-2.5 overflow-y-auto px-3 pb-3">
        <p v-if="historyLoading && !history.length" class="nb-muted py-10 text-center text-sm font-bold">加载中…</p>
        <p v-else-if="!history.length" class="nb-muted py-10 text-center text-sm font-bold">
          {{ prizeFilter === 'win' ? '还没有中奖记录，去幸运九宫格摇一把吧' : '还没有抽奖记录' }}
        </p>
        <div
          v-for="it in history"
          :key="it.id"
          class="nb-line flex items-start gap-3 border-2 p-2.5"
          :class="it.prize_type === 'none' ? 'opacity-60' : 'nb-surface-2'"
          data-testid="slot-history-item"
        >
          <div
            class="nb-line flex size-9 shrink-0 items-center justify-center border-2 text-xl"
            :class="it.prize_type === 'balance' ? 'bg-[#ff2e93]' : it.prize_type === 'none' ? 'nb-surface' : 'bg-[#ffe600]'"
          >{{ it.emoji || '🎁' }}</div>
          <div class="min-w-0 flex-1">
            <div class="flex items-center justify-between gap-2">
              <p class="nb-head truncate text-sm">{{ it.label || '奖品' }}</p>
              <span class="nb-head shrink-0 text-xs tabular-nums">{{ prizeValue(it) }}</span>
            </div>
            <p class="nb-muted text-[11px] font-bold">{{ fmt(it.created_at) }} · 消耗 {{ it.cost }} 积分</p>
            <div v-if="it.redeem_code" class="mt-1.5 flex items-center justify-between gap-2 text-[11px]">
              <code class="truncate font-mono font-black text-[#ff2e93]">{{ it.redeem_code }}</code>
              <button type="button" class="nb-head shrink-0 underline hover:text-[#ff5e00]" @click="copy(it.redeem_code)">复制</button>
            </div>
          </div>
        </div>
      </div>
      <div class="nb-line space-y-2 border-t-2 border-dashed p-3">
        <NbPager :page="historyPage" :total="historyTotal" :page-size="PAGE_SIZE" :disabled="historyLoading" @change="loadHistory" />
        <p class="nb-muted text-[11px] font-bold leading-relaxed">
          积分奖励即时入账；余额奖励已直接加到账户余额，凭证可在「兑换记录」中查到。
        </p>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { getPointsLedger, reasonIcon, reasonLabel, type LedgerItem } from '../points/api'
import { getSlotHistory, type HistoryItem } from '../slot/api'
import NbPager from './NbPager.vue'

const appStore = useAppStore()

type TabKey = 'ledger' | 'prize'
const TABS: { key: TabKey; label: string }[] = [
  { key: 'ledger', label: '🪙 积分流水' },
  { key: 'prize', label: '🎁 奖品记录' }
]
const tab = ref<TabKey>('ledger')

// ─── 积分流水 ──────────────────────────────────────────────────────────
const REASONS = [
  { key: '', label: '全部' },
  { key: 'checkin', label: '签到' },
  { key: 'redeem_bonus', label: '兑换赠送' },
  { key: 'slot_consume', label: '摇奖消耗' },
  { key: 'slot_win', label: '摇奖奖励' },
  { key: 'expire', label: '过期' }
]

// 分页而不是"加载更多"：面板高度固定，不随记录增长
const PAGE_SIZE = 10
const reason = ref('')
const ledger = ref<LedgerItem[]>([])
const ledgerTotal = ref(0)
const ledgerPage = ref(1)
const ledgerLoading = ref(false)

async function loadLedger(page = 1) {
  if (ledgerLoading.value) return
  ledgerLoading.value = true
  try {
    const res = await getPointsLedger({ page, page_size: PAGE_SIZE, reason: reason.value || undefined })
    ledger.value = res.items ?? []
    ledgerTotal.value = res.total
    ledgerPage.value = page
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '积分流水加载失败'))
  } finally {
    ledgerLoading.value = false
  }
}

function setReason(key: string) {
  if (reason.value === key) return
  reason.value = key
  loadLedger(1)
}

// ─── 奖品记录 ──────────────────────────────────────────────────────────
type PrizeFilter = 'win' | 'all'
const PRIZE_FILTERS: { key: PrizeFilter; label: string }[] = [
  { key: 'win', label: '中奖' },
  { key: 'all', label: '全部' }
]
const prizeFilter = ref<PrizeFilter>('win')
const history = ref<HistoryItem[]>([])
const historyTotal = ref(0)
const historyPage = ref(1)
const historyLoading = ref(false)
const historyLoaded = ref(false)

function prizeValue(it: HistoryItem): string {
  if (it.prize_type === 'balance' && it.prize_balance) return `+$${it.prize_balance}`
  if (it.prize_type === 'points' && it.prize_points) return `+${it.prize_points} 积分`
  return it.prize_type === 'none' ? '未中奖' : ''
}

async function loadHistory(page = 1) {
  historyLoading.value = true
  try {
    const res = await getSlotHistory({ page, page_size: PAGE_SIZE, win: prizeFilter.value === 'win' })
    history.value = res.items ?? []
    historyTotal.value = res.total
    historyPage.value = res.page || page
    historyLoaded.value = true
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '奖品记录加载失败'))
  } finally {
    historyLoading.value = false
  }
}

watch(tab, (t) => {
  if (t === 'prize' && !historyLoaded.value) loadHistory()
})
watch(prizeFilter, () => loadHistory(1))

// ─── 公共 ─────────────────────────────────────────────────────────────
function fmt(iso: string): string {
  const d = new Date(iso)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

async function copy(code: string) {
  try {
    await navigator.clipboard.writeText(code)
  } catch {
    /* 非安全上下文下 clipboard 不可用，仍提示让用户手动复制 */
  }
  appStore.showSuccess(`已复制 ${code}`)
}

/** 签到 / 摇奖完成后由父组件调用：刷新已加载过的两张列表 */
function refresh() {
  loadLedger(1)
  if (historyLoaded.value) loadHistory(1)
}

defineExpose({ refresh })

onMounted(() => loadLedger(1))
</script>
