<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
      </div>

      <div v-else-if="!overview?.enabled" class="card py-16 text-center">
        <Icon name="gift" size="xl" class="mx-auto text-gray-300 dark:text-dark-500" />
        <p class="mt-3 text-gray-500 dark:text-gray-400">活动暂未开启，敬请期待</p>
      </div>

      <template v-else>
        <!-- 活动头图 -->
        <div class="card overflow-hidden">
          <div class="bg-gradient-to-r from-orange-500 to-rose-500 px-6 py-6 text-white">
            <h1 class="text-2xl font-bold">兑换刮刮乐</h1>
            <p class="mt-1 text-sm text-white/90">
              每次必中，最高可得
              <span class="font-bold">${{ overview.promo_max_amount.toFixed(2) }}</span
              >，奖金即刻存入余额
            </p>
            <div class="mt-4 flex flex-wrap gap-6 text-sm">
              <div>
                <p class="text-white/70">累计参与</p>
                <p class="text-xl font-semibold">{{ overview.participants.toLocaleString() }} 人</p>
              </div>
              <div>
                <p class="text-white/70">累计送出</p>
                <p class="text-xl font-semibold">
                  ${{
                    overview.total_awarded.toLocaleString(undefined, {
                      minimumFractionDigits: 2,
                      maximumFractionDigits: 2
                    })
                  }}
                </p>
              </div>
            </div>
          </div>
        </div>

        <div class="grid gap-6 lg:grid-cols-5">
          <!-- 刮卡区 -->
          <div class="card flex flex-col items-center p-6 lg:col-span-3">
            <p class="text-sm text-gray-500 dark:text-gray-400">
              剩余刮刮卡
              <span class="ml-1 text-2xl font-bold text-orange-600 dark:text-orange-400" aria-live="polite">{{
                overview.remaining
              }}</span>
              张
            </p>
            <p v-if="overview.remaining > 0 && expiresIn" class="mt-1 text-xs text-gray-400">
              最早一张将在
              <span class="font-medium tabular-nums text-rose-500">{{ expiresIn }}</span>
              后过期
            </p>

            <div class="mt-5 flex min-h-[160px] items-center justify-center">
              <ScratchCard
                v-if="current"
                :key="current.draw_id"
                ref="cardRef"
                :amount="current.amount"
                :big="current.amount >= bigWinThreshold(overview.promo_max_amount)"
                @revealed="onRevealed"
              />
              <div
                v-else
                class="flex h-[160px] w-[300px] items-center justify-center rounded-2xl border-2 border-dashed border-gray-200 text-sm text-gray-400 dark:border-dark-600 dark:text-dark-400"
              >
                {{ overview.remaining > 0 ? '点击下方按钮领取一张卡' : '暂无刮刮卡' }}
              </div>
            </div>

            <div class="mt-5 flex gap-3">
              <button v-if="current && !revealed" class="btn btn-secondary" @click="cardRef?.reveal()">一键刮开</button>
              <button
                v-else-if="overview.remaining > 0"
                class="btn btn-primary min-w-[160px]"
                :disabled="drawing"
                @click="onDraw"
              >
                <span v-if="drawing" class="flex items-center justify-center gap-2">
                  <span class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
                  领取中…
                </span>
                <span v-else>{{ current ? '再刮一张' : '领取并刮开' }}</span>
              </button>
              <button v-else class="btn btn-primary min-w-[160px]" @click="router.push('/redeem')">
                去兑换卡密获取刮刮卡
              </button>
            </div>

            <!-- 规则 -->
            <div
              class="mt-6 w-full rounded-xl bg-gray-50 p-4 text-xs leading-relaxed text-gray-500 dark:bg-dark-800 dark:text-gray-400"
            >
              <p class="mb-1 font-medium text-gray-700 dark:text-gray-300">活动规则</p>
              <ul class="list-disc space-y-0.5 pl-4">
                <li v-for="(tier, i) in sortedTiers" :key="i">
                  兑换单张面值满 ${{ tier.threshold }} 的余额卡密，获得 {{ tier.chances }} 张刮刮卡
                </li>
                <li>
                  刮刮卡自兑换起
                  {{ overview.chance_valid_hours }} 小时内有效，过期作废
                </li>
                <li>每张卡必中，奖金直接存入账户余额，可在余额记录中查看</li>
              </ul>
            </div>
          </div>

          <!-- 动态与排行 -->
          <div class="min-w-0 space-y-6 lg:col-span-2">
            <LotteryLeaderboard v-if="overview.leaderboard_enabled" :items="leaderboard" />
            <LotteryFeed :items="feed" :promo-max="overview.promo_max_amount" />
          </div>
        </div>

        <!-- 我的记录 -->
        <div class="card p-5">
          <div class="flex flex-wrap items-baseline justify-between gap-2">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">我的中奖记录</h2>
            <p v-if="myDraws.length" class="text-xs text-gray-500 dark:text-gray-400">
              近 {{ myDraws.length }} 次共获得
              <span class="font-semibold tabular-nums text-orange-600 dark:text-orange-400"
                >${{ myTotal.toFixed(2) }}</span
              >
              · 单次最高
              <span class="font-semibold tabular-nums text-orange-600 dark:text-orange-400"
                >${{ myBest.toFixed(2) }}</span
              >
            </p>
          </div>
          <div v-if="myDraws.length" class="mt-3 overflow-x-auto">
            <table class="w-full text-sm">
              <thead>
                <tr class="border-b border-gray-100 text-left text-gray-500 dark:border-dark-700 dark:text-gray-400">
                  <th class="py-2 font-medium">时间</th>
                  <th class="py-2 text-right font-medium">金额</th>
                  <th class="py-2 text-right font-medium">状态</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="d in myDraws" :key="d.id" class="border-b border-gray-50 dark:border-dark-800">
                  <td class="py-2 text-gray-600 dark:text-gray-400">
                    {{ formatTime(d.created_at) }}
                  </td>
                  <td class="py-2 text-right font-medium text-gray-900 dark:text-white">${{ d.amount.toFixed(2) }}</td>
                  <td class="py-2 text-right">
                    <span class="text-green-600 dark:text-green-400">已到账</span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else class="mt-3 text-sm text-gray-400">还没有记录</p>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import ScratchCard from './ScratchCard.vue'
import LotteryLeaderboard from './LotteryLeaderboard.vue'
import LotteryFeed from './LotteryFeed.vue'
import { bigWinThreshold } from './feedUtils'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorMessage } from '@/utils/apiError'
import * as lotteryAPI from '@/api/lottery'
import type { DrawResult, FeedItem, LeaderboardItem, LotteryOverview, UserDraw } from '@/api/lottery'

const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()

const loading = ref(true)
const overview = ref<LotteryOverview | null>(null)
const leaderboard = ref<LeaderboardItem[]>([])
const feed = ref<FeedItem[]>([])
const myDraws = ref<UserDraw[]>([])

const current = ref<DrawResult | null>(null)
const revealed = ref(false)
const drawing = ref(false)
const cardRef = ref<InstanceType<typeof ScratchCard> | null>(null)

// 一次“领取”动作对应一个幂等 key：请求失败重试时复用，成功后清空
let pendingKey: string | null = null

// 过期倒计时，每秒刷新
const nowTs = ref(Date.now())
const expiresIn = computed(() => {
  const at = overview.value?.next_expires_at
  if (!at) return ''
  const ms = new Date(at).getTime() - nowTs.value
  if (ms <= 0) return ''
  const h = Math.floor(ms / 3_600_000)
  const m = Math.floor((ms % 3_600_000) / 60_000)
  const s = Math.floor((ms % 60_000) / 1000)
  return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
})

const sortedTiers = computed(() => [...(overview.value?.chance_tiers ?? [])].sort((a, b) => a.threshold - b.threshold))
const myTotal = computed(() => myDraws.value.reduce((a, d) => a + d.amount, 0))
const myBest = computed(() => myDraws.value.reduce((m, d) => Math.max(m, d.amount), 0))

async function loadOverview() {
  overview.value = await lotteryAPI.getOverview()
}

async function loadSide() {
  if (!overview.value?.enabled) return
  const [lb, fd, mine] = await Promise.all([
    overview.value.leaderboard_enabled ? lotteryAPI.getLeaderboard(10) : Promise.resolve([]),
    lotteryAPI.getFeed(30),
    lotteryAPI.getMyDraws(50)
  ])
  leaderboard.value = lb
  feed.value = fd
  myDraws.value = mine
}

async function onDraw() {
  if (drawing.value || !overview.value) return
  drawing.value = true
  pendingKey ??= lotteryAPI.newIdempotencyKey()
  try {
    const res = await lotteryAPI.draw(pendingKey)
    pendingKey = null
    current.value = res
    revealed.value = false
    overview.value.remaining = res.remaining
  } catch (err) {
    const status = (err as { status?: number })?.status
    // 409/403/404 为确定性业务错误，换新 key；网络错误/5xx 保留 key 以便安全重试
    if (status && status < 500) pendingKey = null
    appStore.showError(extractApiErrorMessage(err, '领取失败，请稍后重试'))
    if (status === 409 || status === 403) await loadOverview().catch(() => {})
  } finally {
    drawing.value = false
  }
}

function onRevealed() {
  revealed.value = true
  if (current.value) {
    appStore.showSuccess(`恭喜获得 $${current.value.amount.toFixed(2)}，已存入余额`)
  }
  // 余额和记录在揭晓后刷新，不提前剧透
  authStore.refreshUser?.().catch(() => {})
  loadOverview().catch(() => {})
  loadSide().catch(() => {})
}

function formatTime(s: string) {
  return new Date(s).toLocaleString('zh-CN', {
    hour12: false,
    timeZone: 'Asia/Shanghai'
  })
}

let feedTimer: ReturnType<typeof setInterval> | undefined
let clockTimer: ReturnType<typeof setInterval> | undefined

onMounted(async () => {
  try {
    await loadOverview()
    await loadSide()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, '加载活动失败'))
  } finally {
    loading.value = false
  }
  clockTimer = setInterval(() => {
    const was = expiresIn.value
    nowTs.value = Date.now()
    // 最早一张刚过期：刷新剩余张数与下一张过期时间
    if (was && !expiresIn.value) loadOverview().catch(() => {})
  }, 1000)
  feedTimer = setInterval(() => {
    if (document.visibilityState !== 'visible' || !overview.value?.enabled) return
    lotteryAPI
      .getFeed(30)
      .then((f) => (feed.value = f))
      .catch(() => {})
  }, 60_000)
})

onBeforeUnmount(() => {
  clearInterval(feedTimer)
  clearInterval(clockTimer)
})
</script>
