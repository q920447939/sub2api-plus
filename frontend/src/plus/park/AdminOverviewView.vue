<template>
  <AdminParkPage title="积分乐园概览" subtitle="今日数据按东八区自然日统计" :tabs="[{ key: 'data', label: '概览' }]">
    <template #data>
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex gap-2">
          <button
            v-for="d in [7, 30]"
            :key="d"
            type="button"
            class="btn btn-sm"
            :class="days === d ? 'btn-primary' : 'btn-secondary'"
            @click="days = d"
          >近 {{ d }} 天</button>
        </div>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="load">刷新</button>
      </div>

      <p v-if="error" class="card p-6 text-center text-rose-500">{{ error }}</p>
      <template v-else-if="ci && pt && sl">
        <div class="grid grid-cols-2 gap-4 lg:grid-cols-4" data-testid="park-overview-today">
          <AdminStatTile label="今日签到人数" :value="ci.today.users" :sub="`昨日 ${ci.yesterday.users} 人 · 今日发放 ${ci.today.points} 积分`" />
          <AdminStatTile label="今日发放积分" :value="sum(pt.today_issued)" :sub="breakdown(pt.today_issued)" />
          <AdminStatTile label="今日消耗积分" :value="sum(pt.today_consumed)" :sub="breakdown(pt.today_consumed)" />
          <AdminStatTile label="积分存量" :value="pt.outstanding" :sub="`${pt.accounts} 位用户持有积分`" />
          <AdminStatTile label="今日摇奖次数" :value="sl.today.spins" :sub="`${sl.today.users} 人参与 · 消耗 ${sl.today.cost} 积分`" />
          <AdminStatTile
            label="今日实际返奖率"
            :value="sl.today.cost ? pct(sl.today.payout / sl.today.cost) : '-'"
            :sub="`目标 ${pct(sl.target_rate)}（按配置概率，不含连签加成）`"
            :sub-class="rateClass(sl.today)"
          />
          <AdminStatTile
            label="今日发出余额奖"
            :value="`$${sl.today.prize_balance.toFixed(2)}`"
            :sub="`${sl.today.balance_wins} 次 · 积分奖 ${sl.today.prize_points} 积分`"
          />
          <AdminStatTile label="奖金罐余额" :value="potBalance('small') + potBalance('jackpot')" :sub="`小奖罐 ${potBalance('small')} · 大奖罐 ${potBalance('jackpot')}`" />
        </div>

        <div class="grid gap-4 xl:grid-cols-2">
          <AdminTrendChart
            title="参与趋势"
            :labels="ci.trend.map((d) => d.date)"
            :series="[
              { label: '签到人数', data: ci.trend.map((d) => d.users), color: '#10b981' },
              { label: '摇奖次数', data: sl.trend.map((d) => d.spins), color: '#6366f1' },
              { label: '摇奖人数', data: sl.trend.map((d) => d.users), color: '#f59e0b' }
            ]"
          />
          <AdminTrendChart
            title="积分发放与消耗"
            :labels="pt.trend.map((d) => d.date)"
            :series="[
              { label: '发放', data: pt.trend.map((d) => d.issued), color: '#ec4899' },
              { label: '消耗', data: pt.trend.map((d) => d.consumed), color: '#0ea5e9' },
              { label: '过期', data: pt.trend.map((d) => d.expired), color: '#9ca3af' }
            ]"
          />
        </div>

        <div class="card p-5">
          <p class="mb-3 font-medium text-gray-900 dark:text-white">近 {{ days }} 天合计</p>
          <div class="grid grid-cols-2 gap-4 text-sm md:grid-cols-4">
            <div><p class="text-gray-500">签到人次</p><p class="text-lg font-semibold tabular-nums">{{ ci.trend.reduce((s, d) => s + d.users, 0) }}</p></div>
            <div><p class="text-gray-500">摇奖次数 / 人数</p><p class="text-lg font-semibold tabular-nums">{{ sl.range.spins }} / {{ sl.range.users }}</p></div>
            <div>
              <p class="text-gray-500">实际返奖率</p>
              <p class="text-lg font-semibold tabular-nums">{{ sl.range.cost ? pct(sl.range.payout / sl.range.cost) : '-' }}</p>
            </div>
            <div><p class="text-gray-500">发出余额奖</p><p class="text-lg font-semibold tabular-nums">${{ sl.range.prize_balance.toFixed(2) }}</p></div>
          </div>
        </div>

        <div class="card p-5" data-testid="park-overview-top-holders">
          <div class="mb-3 flex items-center justify-between">
            <div>
              <p class="font-medium text-gray-900 dark:text-white">积分余额排行 Top 10</p>
              <p class="text-xs text-gray-500 dark:text-gray-400">按当前剩余积分排序，不受上方时间范围影响；占比为占全站积分存量的比例</p>
            </div>
            <router-link to="/admin/points-park/points" class="btn btn-secondary btn-sm">全部账户</router-link>
          </div>
          <p v-if="!pt.top_holders?.length" class="py-6 text-center text-sm text-gray-400">暂无用户持有积分</p>
          <div v-else class="overflow-x-auto">
            <table class="w-full min-w-[640px] text-sm">
              <thead class="text-left text-xs text-gray-400">
                <tr>
                  <th class="w-14 py-2">排名</th>
                  <th class="py-2">用户</th>
                  <th class="py-2 text-right">剩余积分</th>
                  <th class="py-2 pl-4">占比</th>
                  <th class="py-2">最近变动</th>
                  <th class="py-2">清零时间</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="h in pt.top_holders" :key="h.user_id">
                  <td class="py-2">
                    <span
                      class="inline-flex size-6 items-center justify-center rounded-full text-xs font-semibold"
                      :class="RANK_CLASS[h.rank - 1] ?? 'text-gray-500'"
                    >{{ h.rank }}</span>
                  </td>
                  <td class="py-2 text-gray-800 dark:text-gray-200">
                    {{ h.email }} <span class="text-xs text-gray-400">#{{ h.user_id }}</span>
                  </td>
                  <td class="py-2 text-right font-semibold tabular-nums">{{ h.balance.toLocaleString() }}</td>
                  <td class="py-2 pl-4">
                    <div class="flex items-center gap-2">
                      <div class="h-1.5 w-24 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                        <div class="h-full rounded-full bg-primary-500" :style="{ width: share(h.balance) }" />
                      </div>
                      <span class="text-xs tabular-nums text-gray-500">{{ share(h.balance) }}</span>
                    </div>
                  </td>
                  <td class="py-2 text-gray-500">{{ fmtDateTime(h.last_active_at) }}</td>
                  <td class="py-2 text-gray-500">{{ h.expire_at ? h.expire_at.slice(0, 10) : '不过期' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </template>
      <p v-else class="card p-6 text-center text-gray-400">加载中…</p>
    </template>
  </AdminParkPage>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminCheckinStats, type CheckinStats } from '../checkin/api'
import { adminPointsStats, reasonLabel, type PointsStats, type ReasonAmount } from '../points/api'
import { adminGetSlotState, adminSlotStats, type PotStat, type SlotStats, type SpinDayStat } from '../slot/api'
import AdminParkPage from './AdminParkPage.vue'
import AdminStatTile from './AdminStatTile.vue'
import AdminTrendChart from './AdminTrendChart.vue'

const days = ref(7)
const loading = ref(false)
const error = ref('')
const ci = ref<CheckinStats | null>(null)
const pt = ref<PointsStats | null>(null)
const sl = ref<SlotStats | null>(null)
const pots = ref<PotStat[]>([])

const sum = (list: ReasonAmount[]) => list.reduce((s, r) => s + r.amount, 0)
const breakdown = (list: ReasonAmount[]) => (list.length ? list.map((r) => `${reasonLabel(r.reason)} ${r.amount}`).join(' · ') : '暂无')
const pct = (v: number) => `${(v * 100).toFixed(1)}%`
const RANK_CLASS = ['bg-amber-400 text-white', 'bg-gray-400 text-white', 'bg-orange-400 text-white']
const share = (bal: number) => (pt.value?.outstanding ? `${((bal / pt.value.outstanding) * 100).toFixed(1)}%` : '-')
function fmtDateTime(iso: string): string {
  const d = new Date(iso)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}
const potBalance = (p: string) => pots.value.find((x) => x.pot === p)?.balance ?? 0

// 样本量够大（≥50 次）且偏离目标超过 10 个百分点时标红
function rateClass(d: SpinDayStat): string {
  if (!sl.value || d.spins < 50 || !d.cost) return ''
  return Math.abs(d.payout / d.cost - sl.value.target_rate) > 0.1 ? 'text-rose-500' : ''
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [a, b, c, st] = await Promise.all([
      adminCheckinStats(days.value),
      adminPointsStats(days.value),
      adminSlotStats(days.value),
      adminGetSlotState()
    ])
    ci.value = a
    pt.value = b
    sl.value = c
    pots.value = st.pots
  } catch (e) {
    error.value = extractApiErrorMessage(e, '统计加载失败')
  } finally {
    loading.value = false
  }
}

watch(days, load)
onMounted(load)
</script>
