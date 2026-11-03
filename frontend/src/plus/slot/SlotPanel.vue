<template>
  <div data-testid="slot-page">
    <div v-if="loading" class="nb-box p-10 text-center font-bold">加载中…</div>
    <div v-else-if="loadError" class="nb-box p-10 text-center">
      <p class="font-bold">{{ loadError }}</p>
      <button type="button" class="nb-btn mt-4 bg-[#ffe600] px-5 py-2 text-sm" @click="load">重试</button>
    </div>
    <div v-else-if="ov && !ov.enabled" class="nb-box p-10 text-center">
      <p class="text-5xl">🛠️</p>
      <p class="nb-head mt-3 text-xl">幸运九宫格暂未开放</p>
      <p class="nb-muted mt-1 text-sm font-bold">敬请期待，先攒点积分吧</p>
      <div v-if="ov.earn_ways?.length" class="mt-5 flex flex-wrap justify-center gap-3">
        <button
          v-for="(w, i) in ov.earn_ways"
          :key="w.key"
          type="button"
          class="nb-btn px-5 py-2 text-sm"
          :class="EARN_COLORS[i % EARN_COLORS.length]"
          @click="earn(w)"
        >{{ earnIcon(w.key) }} {{ w.label }}</button>
      </div>
    </div>

    <!-- 九宫格 | 加成与条件；窄屏上下堆叠 -->
    <div v-else-if="ov" class="grid grid-cols-1 items-start gap-6 lg:grid-cols-[minmax(0,1fr)_260px]">
      <section class="nb-box relative overflow-clip p-4 md:p-6">
        <div class="nb-head nb-faint pointer-events-none absolute -bottom-8 -right-4 rotate-12 select-none text-8xl uppercase opacity-20">JACKPOT</div>
        <div class="relative mb-4 flex flex-wrap items-center justify-between gap-2">
          <p class="text-sm font-bold">
            每次消耗 <span class="nb-line border bg-[#ffe600] px-1 text-black">{{ ov.cost_per_spin }} 积分</span>
            <span class="nb-muted ml-2">今日已摇 {{ ov.spins_today ?? 0 }} 次</span>
          </p>
          <span
            v-if="!ov.can_spin && ov.reason"
            class="nb-tag max-w-full whitespace-normal bg-black px-2 py-1.5 text-[#ffe600]"
            data-testid="slot-blocker"
          >{{ countdown > 0 ? `⏳ ${countdown} 秒后可再摇` : `🚫 ${ov.reason}` }}</span>
          <span v-else class="nb-tag bg-[#ffe600] text-black">准备就绪，点 GO 开摇！</span>
        </div>
        <SlotGrid
          class="relative"
          :tiers="ov.tiers"
          :active="active"
          :cost="ov.cost_per_spin"
          :spinning="spinning"
          :disabled="spinning || !canSpinNow"
          @go="spin"
        />
      </section>

      <aside class="space-y-4" data-testid="slot-status">
        <div class="nb-box p-4">
          <p class="nb-head mb-2 text-sm">连签加成</p>
          <ul v-if="ov.boosts.length" class="space-y-1.5" data-testid="slot-boost-text">
            <li v-for="b in ov.boosts" :key="b.text" class="flex items-start gap-2 text-sm font-bold">
              <span class="nb-line flex size-6 shrink-0 items-center justify-center border-2 bg-[#ff5e00] text-xs">🔥</span>
              <span>{{ b.text }}</span>
            </li>
          </ul>
          <p v-else class="nb-muted text-sm font-bold" data-testid="slot-boost-text">当前连签 {{ ov.streak }} 天，连签达标后大奖概率会提升</p>
        </div>

        <div class="nb-box p-4">
          <p class="nb-head mb-2 text-sm">参与条件</p>
          <ul class="space-y-1.5 text-sm font-bold" data-testid="slot-rules">
            <li class="flex items-start gap-2">
              <span>{{ ov.balance >= ov.cost_per_spin ? '✅' : '❌' }}</span>
              <span>积分 ≥ {{ ov.cost_per_spin }}</span>
            </li>
            <li v-for="r in ov.rules" :key="r.type" class="flex items-start gap-2">
              <span>{{ r.passed ? '✅' : '❌' }}</span><span>{{ r.text }}</span>
            </li>
          </ul>
        </div>

        <!-- 积分获取方式：由后端按当前开启的积分来源返回 -->
        <div v-if="ov.earn_ways?.length" class="nb-box p-4" data-testid="slot-earn-ways">
          <p class="nb-head mb-2 text-sm">{{ ov.blocker === 'insufficient_points' ? '积分不够？这样获取' : '更多积分' }}</p>
          <div class="flex flex-col gap-2">
            <button
              v-for="(w, i) in ov.earn_ways"
              :key="w.key"
              type="button"
              class="nb-btn block px-3 py-2 text-center text-xs"
              :class="EARN_COLORS[i % EARN_COLORS.length]"
              :data-testid="`slot-earn-${w.key}`"
              @click="earn(w)"
            >
              {{ earnIcon(w.key) }} {{ w.label }}
              <span v-if="w.hint" class="block text-[10px] font-bold opacity-80">{{ w.hint }}</span>
            </button>
          </div>
        </div>
      </aside>
    </div>

    <SlotPrizeModal
      :result="result"
      :cost="ov?.cost_per_spin ?? 0"
      :can-again="canSpinNow"
      :cooldown="countdown"
      :again-hint="ov?.reason || '积分不足'"
      @close="result = null"
      @again="again"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import SlotGrid from './SlotGrid.vue'
import SlotPrizeModal from './SlotPrizeModal.vue'
import { useRouter } from 'vue-router'
import { getSlotOverview, newIdempotencyKey, postSpin, type EarnWay, type SlotOverview, type SpinResult } from './api'

const emit = defineEmits<{ changed: []; 'switch-tab': [tab: string] }>()

const appStore = useAppStore()
const router = useRouter()

const EARN_COLORS = ['bg-[#ffe600] text-black', 'bg-[#ff2e93] text-white', 'bg-[#ff5e00] text-white']
const EARN_ICONS: Record<string, string> = { checkin: '📅', redeem_bonus: '🎫' }
const earnIcon = (key: string) => EARN_ICONS[key] ?? '🪙'

/** 积分获取方式的动作：tab:<name> 切换积分乐园标签；route:<path> 跳转 */
function earn(w: EarnWay) {
  const [kind, target] = [w.action.slice(0, w.action.indexOf(':')), w.action.slice(w.action.indexOf(':') + 1)]
  if (kind === 'tab') emit('switch-tab', target)
  else if (kind === 'route') void router.push(target)
}

const loading = ref(true)
const loadError = ref('')
const ov = ref<SlotOverview | null>(null)
const spinning = ref(false)
const active = ref<number | null>(null)
const result = ref<SpinResult | null>(null)
const countdown = ref(0)

const RING = 8
const canSpinNow = computed(() => !!ov.value?.can_spin && countdown.value <= 0)

let timers: number[] = []
let countdownTimer: number | undefined
const sleep = (ms: number) => new Promise<void>((r) => timers.push(window.setTimeout(r, ms)))
const reducedMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false

function startCountdown(sec: number) {
  window.clearInterval(countdownTimer)
  countdown.value = sec
  if (sec <= 0) return
  countdownTimer = window.setInterval(() => {
    countdown.value -= 1
    if (countdown.value <= 0) {
      window.clearInterval(countdownTimer)
      refreshOverview()
    }
  }, 1000)
}

async function refreshOverview() {
  try {
    ov.value = await getSlotOverview()
    startCountdown(ov.value.blocker === 'cooldown' ? ov.value.retry_after ?? 0 : 0)
  } catch {
    /* 静默：保留旧数据 */
  }
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const o = await getSlotOverview()
    ov.value = o
    startCountdown(o.blocker === 'cooldown' ? o.retry_after ?? 0 : 0)
  } catch (e) {
    loadError.value = extractApiErrorMessage(e, '加载失败')
  } finally {
    loading.value = false
  }
}

// 高亮动画：至少 3 圈后减速停在目标格，再闪烁 3 次
async function animateTo(target: number) {
  const start = active.value ?? 0
  if (reducedMotion) {
    active.value = target
    return
  }
  const steps = RING * 3 + ((target - start + RING) % RING)
  for (let i = 1; i <= steps; i++) {
    active.value = (start + i) % RING
    const remain = steps - i
    await sleep(remain < 8 ? 70 + (8 - remain) * 45 : 55)
  }
  for (let i = 0; i < 3; i++) {
    active.value = null
    await sleep(110)
    active.value = target
    await sleep(140)
  }
}

async function spin() {
  if (spinning.value || !canSpinNow.value || !ov.value) return
  spinning.value = true
  result.value = null
  const key = newIdempotencyKey()
  // 请求与预热转圈并行，拿到结果后再收尾，避免空等
  let warm = true
  const warmup = (async () => {
    while (warm && !reducedMotion) {
      active.value = ((active.value ?? -1) + 1) % RING
      await sleep(55)
    }
  })()
  try {
    const res = await postSpin(key)
    warm = false
    await warmup
    ov.value.balance = res.balance
    await animateTo(res.tier_index)
    result.value = res
    emit('changed')
  } catch (e) {
    warm = false
    await warmup
    active.value = null
    appStore.showError(extractApiErrorMessage(e, '摇奖失败，请稍后再试'))
  } finally {
    spinning.value = false
    refreshOverview()
  }
}

function again() {
  result.value = null
  spin()
}

/** 签到等外部操作改变积分后由父组件调用 */
defineExpose({ refresh: refreshOverview })

onMounted(load)
onBeforeUnmount(() => {
  timers.forEach(clearTimeout)
  timers = []
  window.clearInterval(countdownTimer)
})
</script>
