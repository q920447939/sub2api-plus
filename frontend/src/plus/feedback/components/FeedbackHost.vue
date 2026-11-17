<!-- [plus] 反馈中心的全局挂载点：右侧边缘标签 + 首次提示 + 提交面板。AppLayout 只引用这一个组件 -->
<template>
  <div v-if="showTab" class="fb-tab-wrap fixed right-0 top-[62%] z-40 flex items-center">
    <!-- 首次提示：每个浏览器只出现一次，点过入口或关掉后不再出现 -->
    <Transition name="fb-hint">
      <div
        v-if="hint"
        class="mr-2 flex max-w-[220px] items-start gap-2 rounded-xl border border-gray-100 bg-white px-3 py-2.5 text-sm text-gray-700 shadow-card dark:border-dark-700 dark:bg-dark-800 dark:text-dark-100"
        role="status"
      >
        <span>遇到问题或有想法？可以在这里告诉我们</span>
        <button type="button" class="-mr-1 shrink-0 text-gray-400 hover:text-gray-600" aria-label="知道了" @click="dismissHint">
          <Icon name="x" size="sm" />
        </button>
      </div>
    </Transition>

    <button
      type="button"
      class="fb-tab flex w-9 flex-col items-center gap-2 rounded-l-xl border border-r-0 py-3 text-xs font-medium leading-none shadow-card transition-all duration-200"
      aria-label="意见反馈"
      @click="open"
    >
      <svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24">
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M7.5 8.25h9m-9 3H12m-9.75 1.51c0 1.6 1.123 2.994 2.707 3.227 1.129.166 2.27.293 3.423.379.35.026.67.21.865.501L12 21l2.755-4.133a1.14 1.14 0 01.865-.501 48.172 48.172 0 003.423-.379c1.584-.233 2.707-1.626 2.707-3.228V6.741c0-1.602-1.123-2.995-2.707-3.228A48.394 48.394 0 0012 3c-2.392 0-4.744.175-7.043.513C3.373 3.746 2.25 5.14 2.25 6.741v6.018z"
        />
      </svg>
      <span class="flex flex-col items-center gap-0.5"><span>反</span><span>馈</span></span>
    </button>
  </div>
  <Composer v-if="enabled" />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { composer, draft, loadMeta, meta, openComposer, resetDraft } from '../state'
import Composer from './Composer.vue'

const HINT_KEY = 'plus_feedback_hint_seen'
const HINT_DELAY_MS = 6000
const HINT_SHOW_MS = 10000

const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()

const enabled = computed(() => authStore.isAuthenticated && !!meta.value?.enabled)
// 反馈页本身和管理端不显示入口；手机侧边栏打开时也收起，免得浮在遮罩上被误点
const showTab = computed(
  () =>
    enabled.value &&
    !!meta.value?.float_button &&
    !appStore.mobileOpen &&
    !route.path.startsWith('/feedback') &&
    !route.path.startsWith('/admin')
)

// 草稿只属于当前登录用户：同一标签页换号后清掉
watch(
  () => authStore.user?.id,
  (id) => {
    if (id === undefined) return
    if (draft.owner !== undefined && draft.owner !== id) resetDraft(id)
    else draft.owner = id
  },
  { immediate: true }
)

// 返回键或任何跳转都收起面板（草稿在 state 里，不会丢），不把空面板带到下一个页面。
// 每个页面各自挂 AppLayout，换页时这里先被卸载、watch 来不及触发，所以卸载时也收一次
watch(
  () => route.path,
  () => (composer.open = false)
)

const hint = ref(false)
let timers: number[] = []

function dismissHint() {
  hint.value = false
  localStorage.setItem(HINT_KEY, '1')
}

function open() {
  dismissHint()
  openComposer(route.fullPath)
}

// 入口第一次出现后，等用户先看一会页面再轻提示一次，10 秒后自动收起
watch(
  showTab,
  (v) => {
    if (!v || localStorage.getItem(HINT_KEY)) return
    timers.push(
      window.setTimeout(() => {
        if (!showTab.value || localStorage.getItem(HINT_KEY)) return
        hint.value = true
        timers.push(window.setTimeout(dismissHint, HINT_SHOW_MS))
      }, HINT_DELAY_MS)
    )
  },
  { immediate: true }
)

onMounted(() => {
  if (authStore.isAuthenticated) void loadMeta()
})
onBeforeUnmount(() => {
  timers.forEach(clearTimeout)
  timers = []
  composer.open = false
})
</script>

<style scoped>
.fb-tab {
  @apply border-gray-200 bg-white text-gray-600;
  @apply hover:-translate-x-0.5 hover:border-primary-500 hover:bg-primary-500 hover:text-white;
  @apply dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200;
  @apply dark:hover:border-primary-500 dark:hover:bg-primary-500 dark:hover:text-white;
}
.fb-hint-enter-active,
.fb-hint-leave-active {
  transition: all 0.25s ease;
}
.fb-hint-enter-from,
.fb-hint-leave-to {
  opacity: 0;
  transform: translateX(8px);
}
</style>
