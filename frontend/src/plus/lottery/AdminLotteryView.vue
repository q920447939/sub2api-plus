<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <h1 class="text-xl font-semibold text-gray-900 dark:text-white">兑换刮刮乐</h1>
        <span
          v-if="cfg"
          class="rounded-full px-3 py-1 text-xs font-medium"
          :class="
            cfg.enabled
              ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
              : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
          "
          >{{ cfg.enabled ? '活动进行中' : '活动已关闭' }}</span
        >
      </div>

      <!-- Tabs -->
      <div class="border-b border-gray-200 dark:border-dark-700" role="tablist">
        <nav class="-mb-px flex gap-6">
          <button
            v-for="tab in tabs"
            :key="tab.key"
            role="tab"
            :aria-selected="activeTab === tab.key"
            class="border-b-2 px-1 pb-3 text-sm font-medium transition-colors"
            :class="
              activeTab === tab.key
                ? 'border-primary-500 text-primary-600 dark:text-primary-400'
                : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'
            "
            @click="activeTab = tab.key"
          >
            {{ tab.label }}
          </button>
        </nav>
      </div>

      <!-- Tab 1: 统计 + 明细 -->
      <section v-show="activeTab === 'stats'" class="space-y-6">
        <div class="flex flex-wrap items-center gap-3">
          <button class="btn btn-secondary btn-sm" aria-label="前一天" @click="shiftDay(-1)">‹</button>
          <input v-model="day" type="date" class="input w-44" :max="today" />
          <button class="btn btn-secondary btn-sm" aria-label="后一天" :disabled="day >= today" @click="shiftDay(1)">
            ›
          </button>
          <button class="btn btn-secondary btn-sm" :disabled="day === today" @click="day = today">今天</button>
        </div>

        <div v-if="stats" class="grid grid-cols-2 gap-4 md:grid-cols-4">
          <div v-for="m in statCards" :key="m.label" class="card p-4">
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ m.label }}
            </p>
            <p class="mt-1 text-xl font-semibold tabular-nums text-gray-900 dark:text-white">
              {{ m.value }}
            </p>
            <p v-if="m.hint" class="mt-0.5 text-xs text-gray-400">
              {{ m.hint }}
            </p>
          </div>
        </div>

        <div v-if="stats && tierRows.length" class="card p-5">
          <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">奖档分布</h3>
          <div class="space-y-2">
            <div v-for="r in tierRows" :key="r.label" class="flex items-center gap-3 text-sm">
              <span class="w-28 shrink-0 text-gray-600 dark:text-gray-400">{{ r.label }}</span>
              <div class="h-2 flex-1 rounded-full bg-gray-100 dark:bg-dark-700">
                <div class="h-2 rounded-full bg-orange-500" :style="{ width: r.pct + '%' }"></div>
              </div>
              <span class="w-36 shrink-0 text-right tabular-nums text-gray-600 dark:text-gray-400"
                >{{ r.count }} 次 · ${{ r.amount.toFixed(2) }}</span
              >
            </div>
          </div>
        </div>

        <div class="card p-5">
          <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">当日明细</h3>
            <input
              v-model.number="userFilter"
              type="number"
              min="1"
              placeholder="按用户 ID 筛选"
              class="input w-44"
              @keyup.enter="loadDraws(1)"
            />
          </div>
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead>
                <tr class="border-b border-gray-100 text-left text-gray-500 dark:border-dark-700 dark:text-gray-400">
                  <th class="py-2 pr-3 font-medium">时间</th>
                  <th class="py-2 pr-3 font-medium">用户</th>
                  <th class="py-2 pr-3 font-medium">来源卡密</th>
                  <th class="py-2 pr-3 text-right font-medium">卡密面值</th>
                  <th class="py-2 pr-3 text-right font-medium">奖金</th>
                  <th class="py-2 pr-3 font-medium">奖档</th>
                  <th class="py-2 font-medium">备注</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="d in draws" :key="d.id" class="border-b border-gray-50 dark:border-dark-800">
                  <td class="py-2 pr-3 whitespace-nowrap text-gray-600 dark:text-gray-400">
                    {{ formatTime(d.created_at) }}
                  </td>
                  <td class="py-2 pr-3 text-gray-900 dark:text-white">
                    {{ d.display_name }}
                    <span class="text-xs text-gray-400">#{{ d.user_id }}</span>
                  </td>
                  <td class="py-2 pr-3 font-mono text-xs text-gray-600 dark:text-gray-400">
                    {{ d.redeem_code || '#' + d.redeem_code_id }}
                  </td>
                  <td class="py-2 pr-3 text-right tabular-nums">${{ d.code_value.toFixed(2) }}</td>
                  <td class="py-2 pr-3 text-right font-medium tabular-nums text-orange-600 dark:text-orange-400">
                    ${{ d.amount.toFixed(2) }}
                  </td>
                  <td class="py-2 pr-3 text-gray-600 dark:text-gray-400">
                    {{ tierLabel(d.tier_index) }}
                  </td>
                  <td class="py-2 text-xs">
                    <span
                      v-if="d.degraded"
                      class="mr-1 rounded bg-amber-100 px-1.5 py-0.5 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400"
                      >{{ degradedLabel(d.degraded) }}</span
                    >
                  </td>
                </tr>
                <tr v-if="!draws.length">
                  <td colspan="7" class="py-10 text-center text-gray-400">当日暂无记录</td>
                </tr>
              </tbody>
            </table>
          </div>
          <Pagination
            v-if="drawsTotal > pageSize"
            class="mt-4"
            :total="drawsTotal"
            :page="page"
            :page-size="pageSize"
            :show-page-size-selector="false"
            @update:page="loadDraws"
          />
        </div>
      </section>

      <!-- Tab 2: 奖池与规则 -->
      <section v-if="cfg" v-show="activeTab === 'rules'" class="space-y-6">
        <div class="card flex items-center justify-between p-5">
          <div>
            <p class="font-medium text-gray-900 dark:text-white">活动总开关</p>
            <p class="text-xs text-gray-500 dark:text-gray-400">
              只有开启之后兑换的余额卡密才会发卡（自动排除刮刮乐奖励码与管理员调余额）；关闭后用户端所有入口自动隐藏
              <template v-if="cfg.enabled_at"> · 本次开启于 {{ formatTime(cfg.enabled_at) }}</template>
            </p>
          </div>
          <Toggle v-model="cfg.enabled" />
        </div>

        <div class="card space-y-4 p-5">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">发卡阶梯（单张余额卡密面值）</h3>
          <div v-for="(t, i) in cfg.chance_tiers" :key="i" class="flex flex-wrap items-center gap-2 text-sm">
            <span class="text-gray-500">单笔满 $</span>
            <input v-model.number="t.threshold" type="number" min="0.01" step="0.01" class="input w-28" />
            <span class="text-gray-500">得</span>
            <input v-model.number="t.chances" type="number" min="1" step="1" class="input w-20" />
            <span class="text-gray-500">张</span>
            <button class="btn btn-secondary btn-sm" aria-label="删除阶梯" @click="cfg.chance_tiers.splice(i, 1)">
              删除
            </button>
          </div>
          <button class="btn btn-secondary btn-sm" @click="cfg.chance_tiers.push({ threshold: 0, chances: 1 })">
            + 添加阶梯
          </button>
          <label class="flex flex-wrap items-center gap-2 border-t border-gray-100 pt-4 text-sm dark:border-dark-700">
            <span class="text-gray-600 dark:text-gray-400">刮刮卡有效期</span>
            <input v-model.number="cfg.chance_valid_hours" type="number" min="1" step="1" class="input w-24" />
            <span class="text-gray-500">小时，从兑换时刻起算，过期作废</span>
          </label>
          <label class="flex flex-wrap items-center gap-2 text-sm">
            <span class="text-gray-600 dark:text-gray-400">卡密备注关键词</span>
            <input
              v-model.trim="cfg.notes_keyword"
              type="text"
              maxlength="64"
              placeholder="留空 = 全部余额卡密都计入"
              class="input w-64"
            />
            <span class="text-gray-500">填写后只有备注包含该词的卡密才发卡，可用于区分售卖与赠送</span>
          </label>
        </div>

        <div class="card space-y-4 p-5">
          <div>
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">奖池 A · 真实发奖</h3>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              按权重选中奖档后在 [最小, 最大] 内随机取值，真实入账。权重填 0 表示该档永不抽中；
              每张卡必中，最低也会发“保底金额”，想让用户基本只拿小额，把权重集中到最低档即可
            </p>
          </div>
          <PrizePoolEditor ref="poolARef" v-model="cfg.prize_pool_real.tiers" />
        </div>

        <div class="card space-y-4 p-5">
          <div>
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">奖池 B · 虚拟动态</h3>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              只决定虚拟中奖动态与手气榜里虚拟记录的金额，不发钱，档位与区间可与 A 完全不同。
              最高金额不能超过“宣传最高可得”
            </p>
          </div>
          <PrizePoolEditor ref="poolBRef" v-model="cfg.prize_pool_virtual.tiers" />
        </div>

        <div class="card grid gap-4 p-5 md:grid-cols-2">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white md:col-span-2">风控兜底</h3>
          <label class="block text-sm">
            <span class="text-gray-600 dark:text-gray-400">保底金额 $（必中最低值，风控降级时也发此值）</span>
            <input
              v-model.number="cfg.risk_control.floor_amount"
              type="number"
              min="0.01"
              step="0.01"
              class="input mt-1 w-full"
            />
          </label>
          <label class="block text-sm">
            <span class="text-gray-600 dark:text-gray-400">单卡奖金上限（占卡密面值 %）</span>
            <input v-model.number="singleOrderPct" type="number" min="1" max="100" step="1" class="input mt-1 w-full" />
          </label>
          <label class="block text-sm">
            <span class="text-gray-600 dark:text-gray-400">每日总预算 $（0 = 不限；超出后只发保底）</span>
            <input
              v-model.number="cfg.risk_control.daily_budget"
              type="number"
              min="0"
              step="1"
              class="input mt-1 w-full"
            />
          </label>
          <label class="block text-sm">
            <span class="text-gray-600 dark:text-gray-400">单用户每日中奖上限 $（0 = 不限）</span>
            <input
              v-model.number="cfg.risk_control.user_daily_max"
              type="number"
              min="0"
              step="1"
              class="input mt-1 w-full"
            />
          </label>
          <label class="block text-sm md:col-span-2">
            <span class="text-gray-600 dark:text-gray-400">宣传“最高可得” $（展示在用户端，请确保与奖池一致）</span>
            <input
              v-model.number="cfg.promo_max_amount"
              type="number"
              min="0"
              step="0.01"
              class="input mt-1 w-full md:w-1/2"
            />
          </label>
        </div>
      </section>

      <!-- Tab 3: 动态与榜单 -->
      <section v-if="cfg" v-show="activeTab === 'feed'" class="space-y-6">
        <div class="card space-y-4 p-5">
          <div class="flex items-center justify-between">
            <div>
              <p class="font-medium text-gray-900 dark:text-white">虚拟中奖动态</p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                后台每分钟按小时曲线（凌晨少、晚高峰多）和每日 0.7~1.3 倍浮动生成，金额按奖池 B 分布
              </p>
            </div>
            <Toggle v-model="cfg.virtual_feed.enabled" />
          </div>
          <label class="block text-sm">
            <span class="text-gray-600 dark:text-gray-400">每日目标条数（约）</span>
            <input
              v-model.number="cfg.virtual_feed.daily_target"
              type="number"
              min="0"
              step="10"
              class="input mt-1 w-40"
              :disabled="!cfg.virtual_feed.enabled"
            />
          </label>
        </div>

        <div class="card flex items-center justify-between p-5">
          <div>
            <p class="font-medium text-gray-900 dark:text-white">今日手气榜</p>
            <p class="text-xs text-gray-500 dark:text-gray-400">按单次中奖金额取前 10，真实与虚拟记录混排</p>
          </div>
          <Toggle v-model="cfg.virtual_feed.leaderboard_enabled" />
        </div>

        <div class="card grid gap-4 p-5 md:grid-cols-2">
          <div class="md:col-span-2">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">活动头图基数</h3>
            <p class="text-xs text-gray-500 dark:text-gray-400">用户端展示值 = 基数 + 真实累计</p>
          </div>
          <label class="block text-sm">
            <span class="text-gray-600 dark:text-gray-400">累计参与人数基数</span>
            <input
              v-model.number="cfg.virtual_feed.base_total_participants"
              type="number"
              min="0"
              step="1"
              class="input mt-1 w-full"
            />
          </label>
          <label class="block text-sm">
            <span class="text-gray-600 dark:text-gray-400">累计送出金额基数 $</span>
            <input
              v-model.number="cfg.virtual_feed.base_total_awarded"
              type="number"
              min="0"
              step="0.01"
              class="input mt-1 w-full"
            />
          </label>
        </div>
      </section>

      <!-- 保存栏（配置类 tab） -->
      <div
        v-if="cfg && activeTab !== 'stats'"
        class="sticky bottom-0 z-10 flex items-center justify-end gap-3 border-t border-gray-200 bg-white/90 py-3 backdrop-blur dark:border-dark-700 dark:bg-dark-900/90"
      >
        <span v-if="dirty" class="text-xs text-amber-600 dark:text-amber-400">有未保存的修改</span>
        <button class="btn btn-secondary" :disabled="saving || !dirty" @click="resetConfig">撤销</button>
        <button class="btn btn-primary" :disabled="saving || !dirty" @click="saveConfig">
          {{ saving ? '保存中…' : '保存配置' }}
        </button>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import * as lotteryAPI from '@/api/lottery'
import type { AdminDrawRecord, DailyStats, LotteryConfig } from '@/api/lottery'
import PrizePoolEditor from './PrizePoolEditor.vue'

type TabKey = 'stats' | 'rules' | 'feed'
const tabs: { key: TabKey; label: string }[] = [
  { key: 'stats', label: '统计 + 明细' },
  { key: 'rules', label: '奖池与规则' },
  { key: 'feed', label: '动态与榜单' }
]

const appStore = useAppStore()
const activeTab = ref<TabKey>('stats')

// ─── 统计 + 明细 ─────────────────────────────────────────────

function localDay(d = new Date()) {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${dd}`
}

const today = localDay()
const day = ref(today)
const stats = ref<DailyStats | null>(null)
const draws = ref<AdminDrawRecord[]>([])
const drawsTotal = ref(0)
const page = ref(1)
const pageSize = 20
const userFilter = ref<number | ''>('')

function shiftDay(n: number) {
  const [y, m, d] = day.value.split('-').map(Number)
  const next = localDay(new Date(y, m - 1, d + n))
  if (next <= today) day.value = next
}

const money = (n: number) => '$' + n.toFixed(2)

const statCards = computed(() => {
  const s = stats.value
  if (!s) return []
  return [
    {
      label: '刮卡次数',
      value: s.draw_count.toLocaleString(),
      hint: `${s.unique_users} 人参与`
    },
    { label: '发放奖金', value: money(s.total_awarded), hint: '' },
    {
      label: '返奖率',
      value: (s.return_rate * 100).toFixed(2) + '%',
      hint: '发放奖金 ÷ 当日计入卡密面值'
    },
    {
      label: '计入卡密面值',
      value: money(s.redeem_amount),
      hint: `${s.redeem_count} 张卡密得卡`
    },
    { label: '当日发卡', value: s.granted_chances.toLocaleString(), hint: '' },
    {
      label: '当日卡未刮',
      value: s.unused_chances.toLocaleString(),
      hint: '潜在成本'
    },
    {
      label: '当日卡已过期',
      value: s.expired_chances.toLocaleString(),
      hint: ''
    }
  ]
})

const tierRows = computed(() => {
  const dist = stats.value?.tier_distribution ?? {}
  const total = Object.values(dist).reduce((a, r) => a + r.count, 0) || 1
  return Object.entries(dist)
    .sort(([a], [b]) => Number(a) - Number(b))
    .map(([k, r]) => ({
      label: tierLabel(Number(k)),
      count: r.count,
      amount: r.amount,
      pct: (r.count / total) * 100
    }))
})

function tierLabel(i: number) {
  if (i < 0) return '保底'
  const t = cfg.value?.prize_pool_real.tiers[i]
  return t ? `档${i + 1} $${t.min}~${t.max}` : `档${i + 1}`
}

const degradedText: Record<string, string> = {
  daily_budget: '日预算',
  user_daily: '用户日限',
  single_order_pct: '单笔比例',
  pool_filtered: '比例兜底'
}
const degradedLabel = (k: string) => degradedText[k] ?? k

async function loadStats() {
  stats.value = await lotteryAPI.getStats(day.value)
}

async function loadDraws(p = 1) {
  page.value = p
  const res = await lotteryAPI.getDraws({
    day: day.value,
    user_id: userFilter.value || undefined,
    page: p,
    page_size: pageSize
  })
  draws.value = res.items
  drawsTotal.value = res.total
}

async function loadDay() {
  try {
    await Promise.all([loadStats(), loadDraws(1)])
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, '加载统计失败'))
  }
}

watch(day, loadDay)
watch(userFilter, (v) => {
  if (v === '' || v === null) loadDraws(1).catch(() => {})
})

// ─── 配置 ───────────────────────────────────────────────────

const poolARef = ref<InstanceType<typeof PrizePoolEditor> | null>(null)
const poolBRef = ref<InstanceType<typeof PrizePoolEditor> | null>(null)

const cfg = ref<LotteryConfig | null>(null)
const version = ref(0)
const snapshot = ref('')
const saving = ref(false)

const dirty = computed(() => !!cfg.value && JSON.stringify(cfg.value) !== snapshot.value)

const singleOrderPct = computed({
  get: () => Math.round((cfg.value?.risk_control.single_order_pct ?? 0) * 100),
  set: (v: number) => {
    if (cfg.value) cfg.value.risk_control.single_order_pct = v / 100
  }
})

async function loadConfig() {
  const res = await lotteryAPI.getConfig()
  cfg.value = res.config
  version.value = res.version
  snapshot.value = JSON.stringify(res.config)
}

function resetConfig() {
  if (snapshot.value) cfg.value = JSON.parse(snapshot.value)
}

async function saveConfig() {
  if (!cfg.value || saving.value) return
  // 前端校验
  if (poolARef.value?.error) {
    appStore.showError('奖池 A: ' + poolARef.value.error)
    return
  }
  if (poolBRef.value?.error) {
    appStore.showError('奖池 B: ' + poolBRef.value.error)
    return
  }
  if (cfg.value.promo_max_amount > 0) {
    const bMax = cfg.value.prize_pool_virtual.tiers.reduce((m, t) => (t.weight > 0 && t.max > m ? t.max : m), 0)
    if (bMax > cfg.value.promo_max_amount) {
      appStore.showError(
        `奖池 B 最高可抽到 $${bMax.toFixed(2)}，超出宣传最高可得 $${cfg.value.promo_max_amount.toFixed(2)}`
      )
      return
    }
  }
  saving.value = true
  try {
    version.value = await lotteryAPI.saveConfig(cfg.value, version.value)
    // 重新拉取，拿到服务端写入的 enabled_at 等字段
    await loadConfig()
    appStore.showSuccess('配置已保存')
  } catch (err) {
    const status = (err as { status?: number })?.status
    if (status === 409) {
      appStore.showError('配置已被其他管理员修改，已重新加载，请确认后再保存')
      await loadConfig().catch(() => {})
    } else {
      appStore.showError(extractApiErrorMessage(err, '保存失败'))
    }
  } finally {
    saving.value = false
  }
}

// 未保存离开提醒
function beforeUnload(e: BeforeUnloadEvent) {
  if (dirty.value) e.preventDefault()
}
onBeforeRouteLeave(() => !dirty.value || window.confirm('有未保存的修改，确定离开吗？'))

function formatTime(s: string) {
  return new Date(s).toLocaleString()
}

onMounted(async () => {
  window.addEventListener('beforeunload', beforeUnload)
  try {
    await loadConfig()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, '加载配置失败'))
  }
  await loadDay()
})
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))
</script>
