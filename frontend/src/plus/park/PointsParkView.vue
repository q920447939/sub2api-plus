<template>
  <AppLayout>
    <div class="nb-root" data-testid="park-page">
      <div class="nb-caution" />
      <SlotTicker v-if="feed.length" :items="feed" :fallback="[]" />

      <div class="mx-auto max-w-[1440px] space-y-6 px-4 pt-6 md:px-6">
        <!-- 头部：标题 + 积分余额 -->
        <header class="flex flex-wrap items-center justify-between gap-4">
          <div class="flex items-center gap-3">
            <div class="nb-line flex size-12 -rotate-3 items-center justify-center border-4 bg-[#ffe600] text-2xl" style="box-shadow: 5px 5px 0 var(--nb-shadow)">🎡</div>
            <div>
              <h1 class="nb-head nb-shadow-sm nb-line nb-surface inline-block border-2 px-2 py-0.5 text-2xl md:text-3xl">积分乐园</h1>
              <p class="nb-head nb-muted mt-1 text-xs tracking-wider">签到、兑换卡密攒积分 · 幸运九宫格赢积分和余额</p>
            </div>
          </div>
          <div class="nb-line flex items-center gap-3 border-[3px] bg-black px-4 py-2 text-white" style="box-shadow: 5px 5px 0 var(--nb-shadow)">
            <span class="text-2xl">🪙</span>
            <div>
              <p class="nb-head text-[10px] tracking-widest text-[#ffe600]">我的积分</p>
              <p class="nb-head text-2xl tabular-nums leading-none" data-testid="park-balance">{{ points?.balance ?? '-' }}</p>
            </div>
            <span v-if="expireWarn" class="nb-tag bg-[#ff2e93] text-white">⚠️ {{ expireWarn }} 天后清零</span>
          </div>
        </header>

        <!-- 玩法切换 -->
        <nav class="flex flex-wrap gap-3" role="tablist">
          <button
            v-for="t in TABS"
            :key="t.key"
            type="button"
            role="tab"
            :aria-selected="tab === t.key"
            class="nb-btn px-5 py-2.5 text-sm"
            :class="tab === t.key ? 'bg-black text-[#ffe600]' : 'nb-surface nb-hover'"
            :data-testid="`park-tab-${t.key}`"
            @click="setTab(t.key)"
          >{{ t.label }}</button>
        </nav>

        <!-- 玩法 | 记录 -->
        <div class="grid grid-cols-1 items-start gap-6 xl:grid-cols-[minmax(0,1fr)_360px]">
          <div>
            <KeepAlive>
              <CheckinPanel v-if="tab === 'checkin'" @changed="onChanged" />
              <SlotPanel v-else ref="slotPanel" @changed="onChanged" @switch-tab="onSwitchTab" />
            </KeepAlive>
          </div>
          <ParkRecords ref="records" class="xl:sticky xl:top-20" />
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import '../arcade/brutal.css'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import CheckinPanel from '../checkin/CheckinPanel.vue'
import SlotPanel from '../slot/SlotPanel.vue'
import SlotTicker from '../slot/SlotTicker.vue'
import ParkRecords from './ParkRecords.vue'
import { getSlotFeed, type FeedItem } from '../slot/api'
import { getPointsOverview, type PointsOverview } from '../points/api'

type TabKey = 'checkin' | 'grid'
const TABS: { key: TabKey; label: string }[] = [
  { key: 'checkin', label: '📅 每日签到' },
  { key: 'grid', label: '🎰 幸运九宫格' }
]

const route = useRoute()
const router = useRouter()

const tab = computed<TabKey>(() => (route.query.tab === 'grid' ? 'grid' : 'checkin'))
function setTab(key: TabKey) {
  if (key !== tab.value) void router.replace({ query: { ...route.query, tab: key } })
}

function onSwitchTab(key: string) {
  if (TABS.some((t) => t.key === key)) setTab(key as TabKey)
}

const points = ref<PointsOverview | null>(null)
const feed = ref<FeedItem[]>([])
const records = ref<InstanceType<typeof ParkRecords> | null>(null)
const slotPanel = ref<InstanceType<typeof SlotPanel> | null>(null)

const expireWarn = computed(() => {
  const d = points.value?.warn_days
  return d && d > 0 ? d : null
})

async function loadPoints() {
  try {
    points.value = await getPointsOverview()
  } catch {
    /* 余额加载失败不影响玩法 */
  }
}

async function loadFeed() {
  try {
    feed.value = await getSlotFeed()
  } catch {
    /* 跑马灯可选 */
  }
}

// 签到或摇奖后：刷新余额、记录，并让另一个玩法面板拿到最新积分
function onChanged() {
  void loadPoints()
  void loadFeed()
  records.value?.refresh()
  slotPanel.value?.refresh()
}

let feedTimer: number | undefined
onMounted(() => {
  void loadPoints()
  void loadFeed()
  feedTimer = window.setInterval(loadFeed, 60_000)
})
onBeforeUnmount(() => window.clearInterval(feedTimer))
</script>
