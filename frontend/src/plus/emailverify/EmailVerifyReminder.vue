<!-- [plus] 登录后的邮箱未验证提醒：以警告弹窗形式出现。
     - 每次登录都检查：当前邮箱待验证 / 无效就弹（status.remind），服务端不记录用户关闭过
     - 同一次登录内关掉后不再重复弹（刷新页面、切换页面都不弹），退出登录或关闭浏览器后下次登录再弹；
       需要时用户可以在个人资料页「邮箱验证」卡片里随时验证
     - 和签到等其他登录弹窗排队（plus/shared/popupQueue），优先级高于签到；上游公告、新手引导显示时等待
     挂在 App.vue 而不是 AppLayout：AppLayout 每个页面都会重新挂载，放在那里会重复检查。 -->
<template>
  <EmailVerifyModal v-model="open" dismiss-text="下次再说" @update:model-value="onToggle" />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { acquirePopupSlot, POPUP_PRIORITY } from '../shared/popupQueue'
import EmailVerifyModal from './EmailVerifyModal.vue'
import { getEmailVerifyStatus } from './api'

// 本次登录已经提醒过（sessionStorage：关闭浏览器即失效；退出登录时主动清除）
const SHOWN_PREFIX = 'plus:email-verify-reminded:'

function shownThisLogin(uid: number): boolean {
  try {
    return sessionStorage.getItem(SHOWN_PREFIX + uid) === '1'
  } catch {
    return false
  }
}

function markShown(uid: number) {
  try {
    sessionStorage.setItem(SHOWN_PREFIX + uid, '1')
  } catch {
    // 隐私模式写不进去：只是刷新后会再弹一次
  }
}

function clearShown() {
  try {
    for (let i = sessionStorage.length - 1; i >= 0; i--) {
      const k = sessionStorage.key(i)
      if (k?.startsWith(SHOWN_PREFIX)) sessionStorage.removeItem(k)
    }
  } catch {
    // ignore
  }
}

const route = useRoute()
const authStore = useAuthStore()
const userId = computed(() => authStore.user?.id ?? 0)

const open = ref(false)
let release: (() => void) | null = null
let abort: AbortController | null = null

async function shouldRemind(): Promise<boolean> {
  try {
    return (await getEmailVerifyStatus()).remind
  } catch {
    return false
  }
}

async function tryOpen(uid: number) {
  abort?.abort()
  const ctl = new AbortController()
  abort = ctl
  if (shownThisLogin(uid)) return
  if (!(await shouldRemind()) || ctl.signal.aborted) return
  // 公开页（首页、文档等）不打扰，等进入需要登录的页面
  while (!ctl.signal.aborted && route.meta.requiresAuth === false) {
    await new Promise((r) => setTimeout(r, 1000))
  }
  const r = await acquirePopupSlot('email-verify', POPUP_PRIORITY.warning, ctl.signal)
  if (!r) return
  // 排队期间可能已在个人资料页完成验证、退出登录或进入公开页
  if (ctl.signal.aborted || authStore.user?.id !== uid || route.meta.requiresAuth === false || !(await shouldRemind())) {
    r()
    return
  }
  release = r
  markShown(uid)
  open.value = true
}

// 弹窗关闭（验证成功或用户关掉）时归还显示权，排在后面的签到弹窗才会出来
function onToggle(v: boolean) {
  if (!v && release) {
    release()
    release = null
  }
}

watch(
  userId,
  (uid, prev) => {
    open.value = false
    onToggle(false)
    if (prev && prev !== uid) clearShown() // 退出 / 切换账号：下次登录重新提醒
    if (!uid) {
      abort?.abort()
      return
    }
    void tryOpen(uid)
  },
  { immediate: true }
)

onBeforeUnmount(() => {
  abort?.abort()
  release?.()
})
</script>
