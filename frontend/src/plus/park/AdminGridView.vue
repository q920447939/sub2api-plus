<template>
  <AdminParkPage
    title="幸运九宫格"
    :tabs="[
      { key: 'data', label: '摇奖数据' },
      { key: 'config', label: '奖品与规则' },
      { key: 'atmosphere', label: '人气氛围' }
    ]"
    :config-tabs="['config', 'atmosphere']"
    section="slot"
    save-label="保存九宫格配置"
  >
    <template #data>
      <div class="flex gap-2">
        <button
          v-for="d in [1, 7, 30]"
          :key="d"
          type="button"
          class="btn btn-sm"
          :class="days === d ? 'btn-primary' : 'btn-secondary'"
          @click="days = d"
        >{{ d === 1 ? '今天' : `近 ${d} 天` }}</button>
      </div>

      <template v-if="stats">
        <div class="grid grid-cols-2 gap-4 lg:grid-cols-4" data-testid="grid-admin-stats">
          <AdminStatTile label="摇奖次数" :value="stats.range.spins" :sub="`${stats.range.users} 人参与`" />
          <AdminStatTile label="消耗积分" :value="stats.range.cost" :sub="`奖金罐支出 ${stats.range.payout}`" />
          <AdminStatTile
            label="实际返奖率"
            :value="stats.range.cost ? pct(stats.range.payout / stats.range.cost) : '-'"
            :sub="`目标 ${pct(stats.target_rate)}（按配置概率，不含连签加成）`"
          />
          <AdminStatTile
            label="发出余额奖"
            :value="`$${stats.range.prize_balance.toFixed(2)}`"
            :sub="`${stats.range.balance_wins} 次 · 积分奖 ${stats.range.prize_points} 积分`"
          />
        </div>

        <AdminTrendChart
          v-if="days > 1"
          title="每日摇奖"
          :labels="stats.trend.map((d) => d.date ?? '')"
          :series="[
            { label: '摇奖次数', data: stats.trend.map((d) => d.spins), color: '#6366f1' },
            { label: '参与人数', data: stats.trend.map((d) => d.users), color: '#f59e0b' }
          ]"
        />

        <div class="card p-5">
          <p class="font-medium text-gray-900 dark:text-white">各奖档实际中出</p>
          <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">
            配置概率为当前配置；实际中出受连签加成、奖金罐不足排除影响，样本少时波动大属正常
          </p>
          <div class="overflow-x-auto">
            <table class="w-full min-w-[560px] text-sm">
              <thead class="text-left text-xs text-gray-400">
                <tr>
                  <th class="py-2">奖品</th>
                  <th class="py-2 text-right">配置概率</th>
                  <th class="py-2 text-right">实际中出</th>
                  <th class="py-2 text-right">实际占比</th>
                  <th class="py-2 text-right">因奖金罐不足被排除</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="t in stats.tiers" :key="t.index">
                  <td class="py-2">{{ t.emoji }} {{ t.label }}</td>
                  <td class="py-2 text-right tabular-nums">{{ fmtBP(t.prob_bp) }}</td>
                  <td class="py-2 text-right tabular-nums">{{ t.hits }}</td>
                  <td class="py-2 text-right tabular-nums" :class="deviates(t) ? 'text-rose-500' : ''">
                    {{ stats.range.spins ? pct(t.hits / stats.range.spins) : '-' }}
                  </td>
                  <td class="py-2 text-right tabular-nums text-gray-500">{{ t.excluded || '' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </template>

      <div class="card p-5">
        <div class="mb-3 flex flex-wrap items-end gap-3">
          <div class="mr-auto">
            <p class="font-medium text-gray-900 dark:text-white">摇奖记录</p>
            <p class="text-xs text-gray-500 dark:text-gray-400">“被排除”是这次因奖金罐余额不足而抽不到的奖品</p>
          </div>
          <select v-model="filter.prize_type" class="input w-32" @change="reload">
            <option value="">全部结果</option>
            <option value="points">积分奖</option>
            <option value="balance">余额奖</option>
            <option value="none">未中奖</option>
          </select>
          <input v-model="filter.date" type="date" class="input w-40" @change="reload" />
          <input v-model="filter.q" class="input w-56" placeholder="邮箱 / 用户名 / 用户 ID" @keyup.enter="reload" />
          <button type="button" class="btn btn-secondary btn-sm" @click="reload">查询</button>
        </div>
        <div class="overflow-x-auto">
          <table class="w-full min-w-[760px] text-sm">
            <thead class="text-left text-xs text-gray-400">
              <tr>
                <th class="py-2">时间</th>
                <th class="py-2">用户</th>
                <th class="py-2">结果</th>
                <th class="py-2 text-right">消耗</th>
                <th class="py-2 text-right">扣罐</th>
                <th class="py-2 pl-3">加成</th>
                <th class="py-2">被排除</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="s in spins" :key="s.id">
                <td class="py-2 text-gray-500">{{ fmtTime(s.created_at) }}</td>
                <td class="py-2">{{ s.email || `#${s.user_id}` }}</td>
                <td class="py-2" :class="s.prize_type === 'none' ? 'text-gray-400' : ''">{{ prizeText(s) }}</td>
                <td class="py-2 text-right tabular-nums">{{ s.cost }}</td>
                <td class="py-2 text-right tabular-nums">{{ s.pot_payout ? `${s.pot === 'jackpot' ? '大' : '小'} -${s.pot_payout}` : '' }}</td>
                <td class="py-2 pl-3">{{ s.boosted ? '🔥' : '' }}</td>
                <td class="py-2 text-xs text-gray-500">{{ (s.excluded_tiers ?? []).map(tierLabel).join('、') }}</td>
              </tr>
              <tr v-if="!spins.length">
                <td colspan="7" class="py-8 text-center text-gray-400">{{ loading ? '加载中…' : '暂无记录' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <Pagination
          v-if="total > 0"
          class="mt-3"
          :total="total"
          :page="page"
          :page-size="pageSize"
          @update:page="(p) => loadSpins(p)"
          @update:page-size="(s) => { pageSize = s; loadSpins(1) }"
        />
      </div>
    </template>

    <template #config>
      <AdminGridTab />
    </template>
    <template #atmosphere>
      <AdminAtmosphereTab />
    </template>
  </AdminParkPage>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import Pagination from '@/components/common/Pagination.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminSlotStats, adminSpins, fmtBP, type AdminSpin, type PrizeType, type SlotStats, type TierStat } from '../slot/api'
import AdminAtmosphereTab from './AdminAtmosphereTab.vue'
import AdminGridTab from './AdminGridTab.vue'
import AdminParkPage from './AdminParkPage.vue'
import AdminStatTile from './AdminStatTile.vue'
import AdminTrendChart from './AdminTrendChart.vue'
import { fmtTime } from './adminFormat'

const appStore = useAppStore()
const pct = (v: number) => `${(v * 100).toFixed(1)}%`

const days = ref(7)
const stats = ref<SlotStats | null>(null)

async function loadStats() {
  try {
    stats.value = await adminSlotStats(days.value)
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '摇奖统计加载失败'))
  }
}

// 样本 ≥200 次且实际占比偏离配置超过一半时标红，提示检查奖金罐或配置
function deviates(t: TierStat): boolean {
  const n = stats.value?.range.spins ?? 0
  if (n < 200 || t.prob_bp === 0) return false
  const want = t.prob_bp / 10000
  return Math.abs(t.hits / n - want) > want / 2
}

function tierLabel(i: number): string {
  const t = stats.value?.tiers[i]
  return t ? `${t.emoji ?? ''} ${t.label}`.trim() : `第 ${i + 1} 档`
}

function prizeText(s: AdminSpin): string {
  if (s.prize_type === 'points') return `${tierLabel(s.tier_index)}（+${s.prize_points} 积分）`
  if (s.prize_type === 'balance') return `${tierLabel(s.tier_index)}（+$${s.prize_balance}）`
  return tierLabel(s.tier_index)
}

const filter = reactive({ q: '', date: '', prize_type: '' as PrizeType | '' })
const spins = ref<AdminSpin[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)

async function loadSpins(p = page.value) {
  loading.value = true
  try {
    const res = await adminSpins({ page: p, page_size: pageSize.value, q: filter.q.trim(), date: filter.date, prize_type: filter.prize_type })
    spins.value = res.items ?? []
    total.value = res.total
    page.value = res.page || p
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '摇奖记录加载失败'))
  } finally {
    loading.value = false
  }
}
const reload = () => loadSpins(1)

watch(days, loadStats)
onMounted(() => {
  void loadStats()
  void loadSpins(1)
})
</script>
