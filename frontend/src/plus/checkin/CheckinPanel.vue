<template>
  <div class="space-y-6" data-testid="checkin-page">
    <div v-if="loading" class="nb-box p-10 text-center font-bold">加载中…</div>
    <div v-else-if="loadError" class="nb-box p-10 text-center">
      <p class="font-bold">{{ loadError }}</p>
      <button type="button" class="nb-btn mt-4 bg-[#ffe600] px-5 py-2 text-sm" @click="load">重试</button>
    </div>
    <div v-else-if="status && !status.enabled" class="nb-box p-10 text-center">
      <p class="text-5xl">🛠️</p>
      <p class="nb-head mt-3 text-xl">签到活动暂未开放</p>
    </div>

    <template v-else-if="status">
      <!-- 连签 + 签到按钮 -->
      <section class="nb-box-lg overflow-hidden">
        <div class="nb-line flex flex-wrap items-center justify-between gap-3 border-b-4 bg-black p-5 text-white">
          <div>
            <p class="nb-head text-[11px] uppercase tracking-widest text-[#ffe600]">每日签到</p>
            <p class="nb-head mt-1 text-2xl">{{ status.today_checked ? '今日已签到 ✓' : `今日可领 +${status.today_points} 积分` }}</p>
          </div>
          <div class="text-right">
            <span class="nb-tag bg-[#ff5e00] text-white" data-testid="checkin-streak">🔥 连签 {{ status.streak }} 天</span>
            <p class="nb-faint mt-2 text-xs font-bold">{{ streakHint }}</p>
          </div>
        </div>

        <!-- 7 天进度 -->
        <div class="p-5">
          <p class="nb-head mb-3 text-xs uppercase tracking-wider nb-muted">本轮 7 天进度</p>
          <CheckinWeek :status="status" />
        </div>

        <div class="px-5 pb-5">
          <button
            type="button"
            class="nb-btn w-full bg-[#ff2e93] py-4 text-lg text-white hover:bg-[#ff1484]"
            style="border-width: 4px"
            :disabled="status.today_checked || submitting"
            data-testid="checkin-submit"
            @click="doCheckin"
          >
            <template v-if="status.today_checked">✓ 今日已签到</template>
            <template v-else-if="submitting">签到中…</template>
            <template v-else>签到领取 +{{ status.today_points }} 积分</template>
          </button>
          <p v-if="checkinError" class="mt-2 text-center text-sm font-bold text-[#ff2e93]" role="alert">{{ checkinError }}</p>
        </div>
      </section>

      <!-- 月历 -->
      <section class="nb-box p-5">
        <div class="mb-4 flex items-center justify-between">
          <h2 class="nb-head text-lg">{{ calendarTitle }}</h2>
          <div class="flex gap-2">
            <button type="button" class="nb-btn size-8 nb-surface text-sm" style="box-shadow: 3px 3px 0 var(--nb-shadow)" aria-label="上个月" @click="shiftMonth(-1)">‹</button>
            <button type="button" class="nb-btn size-8 nb-surface text-sm" style="box-shadow: 3px 3px 0 var(--nb-shadow)" aria-label="下个月" :disabled="isCurrentMonth" @click="shiftMonth(1)">›</button>
          </div>
        </div>
        <div class="nb-head mb-2 grid grid-cols-7 text-center text-xs nb-muted">
          <span v-for="d in ['日', '一', '二', '三', '四', '五', '六']" :key="d">{{ d }}</span>
        </div>
        <div class="grid grid-cols-7 gap-1.5">
          <div v-for="n in leadingBlanks" :key="'b' + n" />
          <div
            v-for="day in daysInMonth"
            :key="day"
            class="flex h-9 items-center justify-center border-2 text-sm font-bold"
            :class="dayClass(day)"
          >{{ day }}</div>
        </div>
        <p class="mt-4 text-xs font-bold nb-muted">
          每 7 天一轮循环，第 7 天额外 +{{ status.week_bonus }}；断签后从第 1 天重新计。连签越久，幸运九宫格的大奖概率越高。
        </p>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { getCheckinStatus, postCheckin, type CheckinStatus } from './api'
import CheckinWeek from './CheckinWeek.vue'
import { checkinTodayPos } from './week'
import { checkinVersion, notifyCheckedIn } from './popupState'

const appStore = useAppStore()
const emit = defineEmits<{ changed: [] }>()

const loading = ref(true)
const loadError = ref('')
const submitting = ref(false)
const checkinError = ref('')
const status = ref<CheckinStatus | null>(null)
const month = ref('') // YYYY-MM，空表示当月
const monthChecked = ref<string[]>([])

const todayPos = computed(() => (status.value ? checkinTodayPos(status.value) : 1))

const streakHint = computed(() => {
  const s = status.value
  if (!s) return ''
  const left = 7 - todayPos.value + (s.today_checked ? 0 : 1)
  if (s.today_checked && todayPos.value === 7) return '🏆 本轮满签达成！'
  return `再签 ${left} 天领满签奖励`
})

const calMonth = computed(() => month.value || status.value?.month || status.value?.today.slice(0, 7) || '')
const isCurrentMonth = computed(() => calMonth.value === status.value?.today.slice(0, 7))
const calendarTitle = computed(() => {
  const [y, m] = calMonth.value.split('-')
  return `${y} 年 ${Number(m)} 月`
})
const daysInMonth = computed(() => {
  const [y, m] = calMonth.value.split('-').map(Number)
  return new Date(y, m, 0).getDate()
})
const leadingBlanks = computed(() => {
  const [y, m] = calMonth.value.split('-').map(Number)
  return new Date(y, m - 1, 1).getDay()
})
const checkedSet = computed(() => new Set(monthChecked.value))

function dayClass(day: number): string {
  const date = `${calMonth.value}-${String(day).padStart(2, '0')}`
  const today = status.value?.today ?? ''
  if (checkedSet.value.has(date)) return 'nb-line bg-[#ffe600]'
  if (date === today) return 'nb-line nb-surface outline outline-2 outline-[#ff2e93]'
  return date > today ? 'border-transparent nb-faint' : 'border-transparent nb-muted'
}

async function shiftMonth(delta: number) {
  const [y, m] = calMonth.value.split('-').map(Number)
  const d = new Date(y, m - 1 + delta, 1)
  const next = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
  try {
    const st = await getCheckinStatus(next)
    month.value = st.month
    monthChecked.value = st.month_checked
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '加载失败'))
  }
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const st = await getCheckinStatus()
    status.value = st
    month.value = st.month
    monthChecked.value = st.month_checked
  } catch (e) {
    loadError.value = extractApiErrorMessage(e, '加载失败')
  } finally {
    loading.value = false
  }
}

async function doCheckin() {
  if (!status.value || status.value.today_checked || submitting.value) return
  submitting.value = true
  checkinError.value = ''
  try {
    const res = await postCheckin()
    appStore.showSuccess(`签到成功！获得 ${res.points_granted} 积分`)
    notifyCheckedIn()
    emit('changed')
    await load()
  } catch (e) {
    checkinError.value = extractApiErrorMessage(e, '签到失败，请稍后重试')
  } finally {
    submitting.value = false
  }
}

onMounted(load)

// 顶栏弹窗里签到后，本页同步刷新
watch(checkinVersion, () => {
  if (submitting.value) return
  emit('changed')
  void load()
})
</script>
