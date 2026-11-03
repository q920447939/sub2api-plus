<template>
  <Teleport to="body">
    <Transition name="nb-fade">
      <div
        v-if="popupOpen && status"
        class="nb-root fixed inset-0 z-[90] m-0 flex min-h-0 items-center justify-center bg-black/70 p-4 backdrop-blur-sm"
        style="background-image: none"
        role="dialog"
        aria-modal="true"
        aria-labelledby="plus-checkin-popup-title"
        data-testid="checkin-popup"
        @click.self="close"
      >
        <Transition name="nb-pop" appear>
          <div class="nb-box-xl relative w-full max-w-md overflow-hidden">
            <button
              type="button"
              class="absolute right-3 top-3 z-10 flex size-8 items-center justify-center border-2 nb-line nb-surface text-lg font-black"
              aria-label="关闭"
              data-testid="checkin-popup-close"
              @click="close"
            >×</button>

            <div class="border-b-4 nb-line bg-black px-5 pb-4 pt-5 text-white">
              <p class="nb-head text-[11px] uppercase tracking-widest text-[#ffe600]">DAILY CHECK-IN</p>
              <h2 id="plus-checkin-popup-title" class="nb-head mt-1 text-2xl">
                {{ result ? '签到成功' : '今日还没签到' }}
              </h2>
              <p class="mt-1 text-xs font-bold text-white/70">
                <span class="nb-tag bg-[#ff5e00] text-white">🔥 连签 {{ status.streak }} 天</span>
                <span class="ml-2">余额 {{ status.balance }} 积分</span>
              </p>
            </div>

            <div class="space-y-4 p-5">
              <CheckinWeek :status="status" />

              <div v-if="result" class="nb-box nb-cream p-4 text-center" data-testid="checkin-popup-result">
                <p class="nb-head text-3xl text-[#ff2e93]">+{{ result.points_granted }} 积分</p>
                <p class="mt-1 text-xs font-bold nb-muted">当前余额 {{ result.balance }} 积分 · 明天继续来</p>
              </div>

              <template v-if="result">
                <div class="flex flex-col gap-3">
                  <button type="button" class="nb-btn w-full bg-[#ffe600] py-3 text-sm hover:bg-yellow-300" data-testid="checkin-popup-slot" @click="goSlot">
                    🎰 去幸运九宫格试试手气
                  </button>
                  <button type="button" class="nb-btn w-full nb-surface py-2.5 text-xs nb-hover" @click="close">关闭</button>
                </div>
              </template>
              <template v-else>
                <button
                  type="button"
                  class="nb-btn w-full bg-[#ff2e93] py-4 text-lg text-white hover:bg-[#ff1484]"
                  style="border-width: 4px"
                  :disabled="submitting"
                  data-testid="checkin-popup-submit"
                  @click="doCheckin"
                >{{ submitting ? '签到中…' : `签到领取 +${status.today_points} 积分` }}</button>
                <p v-if="error" class="text-center text-sm font-bold text-[#ff2e93]" role="alert">{{ error }}</p>
                <div class="flex items-center justify-between gap-3">
                  <label class="flex cursor-pointer select-none items-center gap-2 text-xs font-bold nb-muted">
                    <input v-model="muteToday" type="checkbox" class="size-4 accent-[#ff2e93]" data-testid="checkin-popup-mute" />
                    今日不再弹出
                  </label>
                  <button type="button" class="nb-btn nb-surface px-4 py-1.5 text-xs nb-hover" data-testid="checkin-popup-later" @click="close">稍后</button>
                </div>
              </template>
            </div>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
// [plus] 签到弹窗：登录后自动弹出（今日未签 + 后台开关开启），也可由顶栏按钮手动打开。
import '../arcade/brutal.css'
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { CheckinResult } from './api'
import CheckinWeek from './CheckinWeek.vue'
import {
  checkinStatus,
  dismissForSession,
  isPopupSuppressed,
  muteForToday,
  popupOpen,
  refreshCheckinStatus,
  resetCheckinPopupSession,
  submitCheckin
} from './popupState'
import { waitForQuietScreen } from '../shared/waitForQuietScreen'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

const status = checkinStatus
const submitting = ref(false)
const error = ref('')
const result = ref<CheckinResult | null>(null)
const muteToday = ref(false)

const userId = computed(() => authStore.user?.id ?? 0)

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') close()
}

// 打开时重置弹窗内状态
watch(popupOpen, (open) => {
  if (!open) {
    window.removeEventListener('keydown', onKeydown)
    return
  }
  window.addEventListener('keydown', onKeydown)
  result.value = null
  error.value = ''
  muteToday.value = false
  void nextTick(() => {
    document.querySelector<HTMLElement>('[data-testid="checkin-popup-submit"]')?.focus()
  })
})

function close() {
  if (!popupOpen.value) return
  const uid = userId.value
  // 签到完成后关闭无需记录；未签时关闭按勾选记录
  if (uid && !result.value && status.value) {
    if (muteToday.value) muteForToday(uid, status.value.today)
    else dismissForSession(uid)
  }
  popupOpen.value = false
}

async function doCheckin() {
  if (submitting.value) return
  submitting.value = true
  error.value = ''
  try {
    result.value = await submitCheckin()
  } catch (e) {
    error.value = extractApiErrorMessage(e, '签到失败，请稍后重试')
    // 可能是别的标签页已签，刷新一下
    const st = await refreshCheckinStatus()
    if (st?.today_checked) popupOpen.value = false
  } finally {
    submitting.value = false
  }
}

function goSlot() {
  popupOpen.value = false
  void router.push({ path: '/points-park', query: { tab: 'grid' } })
}

// ─── 登录后自动弹出 ──────────────────────────────────────────────

let abort: AbortController | null = null

async function tryAutoOpen(uid: number) {
  abort?.abort()
  const ctl = new AbortController()
  abort = ctl
  const st = await refreshCheckinStatus()
  if (ctl.signal.aborted || !st) return
  if (!st.enabled || !st.popup_enabled || st.today_checked) return
  if (isPopupSuppressed(uid, st.today)) return
  // 只在需要登录的页面弹，公开页（首页、文档等）不打扰
  while (!ctl.signal.aborted && route.meta.requiresAuth === false) {
    await new Promise((r) => setTimeout(r, 1000))
  }
  if (!(await waitForQuietScreen({ signal: ctl.signal }))) return
  if (route.meta.requiresAuth === false) return
  // 等待期间可能已在别处签到或被关闭
  if (ctl.signal.aborted || authStore.user?.id !== uid) return
  if (checkinStatus.value?.today_checked || isPopupSuppressed(uid, st.today)) return
  popupOpen.value = true
}

watch(
  userId,
  (uid, prev) => {
    if (prev && prev !== uid) resetCheckinPopupSession()
    if (!uid) {
      abort?.abort()
      return
    }
    void tryAutoOpen(uid)
  },
  { immediate: true }
)

onBeforeUnmount(() => {
  abort?.abort()
  window.removeEventListener('keydown', onKeydown)
})
</script>
