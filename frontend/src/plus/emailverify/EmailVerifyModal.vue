<!-- [plus] 邮箱验证弹窗：沿用项目统一的 BaseDialog 样式，未验证时以警告形式提示。
     登录后自动弹出（EmailVerifyReminder）和个人资料页手动打开共用这一个组件。 -->
<template>
  <BaseDialog :show="open" :title="title" width="narrow" @close="close">
    <div class="space-y-4" data-testid="email-verify-modal">
      <!-- 无效邮箱 -->
      <div v-if="status?.status === 'invalid'" class="flex gap-3 rounded-xl border border-red-200 bg-red-50 p-4 dark:border-red-800/50 dark:bg-red-900/20">
        <Icon name="exclamationCircle" size="md" class="mt-0.5 shrink-0 text-red-500" />
        <div class="text-sm text-red-700 dark:text-red-300">
          <p class="font-medium">邮箱 {{ status.email }} 无法接收邮件</p>
          <p class="mt-1 text-xs">请先在个人资料里更换为常用邮箱，再完成验证。</p>
        </div>
      </div>

      <template v-else-if="step === 'send'">
        <!-- 警告：未验证的影响 -->
        <div class="flex gap-3 rounded-xl border border-amber-200 bg-amber-50 p-4 dark:border-amber-800/50 dark:bg-amber-900/20">
          <Icon name="exclamationTriangle" size="md" class="mt-0.5 shrink-0 text-amber-500" />
          <div class="min-w-0 text-sm text-amber-800 dark:text-amber-200">
            <p class="font-medium">你的邮箱还没有验证</p>
            <ul class="mt-1.5 list-disc space-y-0.5 pl-4 text-xs text-amber-700 dark:text-amber-300">
              <li>收不到余额不足、订阅到期等重要提醒</li>
              <li>忘记密码时可能无法找回账号</li>
            </ul>
          </div>
        </div>
        <p class="text-sm text-gray-600 dark:text-dark-300">
          验证码将发送到 <span class="font-medium text-gray-900 dark:text-white">{{ status?.email }}</span>
        </p>
        <div v-if="status?.reward_eligible" class="flex items-center gap-2 rounded-xl bg-primary-50 px-4 py-2.5 text-sm text-primary-700 dark:bg-primary-900/20 dark:text-primary-300">
          <Icon name="gift" size="sm" class="shrink-0" />
          <span>验证成功即得 <strong>{{ status.reward_points }} 积分</strong></span>
        </div>
        <p v-if="error" class="text-sm text-red-600 dark:text-red-400" role="alert">{{ error }}</p>
      </template>

      <template v-else-if="step === 'confirm'">
        <p class="text-sm text-gray-600 dark:text-dark-300">
          验证码已发送至 <span class="font-medium text-gray-900 dark:text-white">{{ status?.email }}</span>，请在邮箱中查收（也看看垃圾邮件）。
        </p>
        <div>
          <label class="input-label">验证码</label>
          <input
            ref="codeInput"
            v-model="code"
            type="text"
            inputmode="numeric"
            maxlength="8"
            placeholder="请输入验证码"
            class="input text-center text-lg tracking-widest"
            @keydown.enter="doConfirm"
          />
        </div>
        <p v-if="error" class="text-sm text-red-600 dark:text-red-400" role="alert">{{ error }}</p>
        <button type="button" class="text-xs text-primary-600 hover:underline disabled:text-gray-400 disabled:no-underline" :disabled="cooldown > 0 || sending" @click="doSendCode">
          {{ cooldown > 0 ? `${cooldown} 秒后可重新发送` : '没收到？重新发送' }}
        </button>
      </template>

      <template v-else-if="step === 'success'">
        <div class="flex flex-col items-center py-4 text-center">
          <div class="flex h-12 w-12 items-center justify-center rounded-full bg-green-100 text-green-600 dark:bg-green-900/30 dark:text-green-400">
            <Icon name="check" size="lg" />
          </div>
          <p class="mt-3 text-base font-medium text-gray-900 dark:text-white">邮箱验证成功</p>
          <p v-if="rewardGranted" class="mt-1 text-sm text-gray-600 dark:text-dark-300">已发放 <strong class="text-primary-600">{{ rewardPoints }}</strong> 积分</p>
        </div>
      </template>
    </div>

    <template #footer>
      <template v-if="status?.status === 'invalid'">
        <button type="button" class="btn btn-secondary" @click="close">{{ dismissText }}</button>
        <button type="button" class="btn btn-primary" @click="goProfile">去更换邮箱</button>
      </template>
      <template v-else-if="step === 'send'">
        <button type="button" class="btn btn-secondary" @click="close">{{ dismissText }}</button>
        <button type="button" class="btn btn-primary" :disabled="sending || cooldown > 0" @click="doSendCode">
          {{ cooldown > 0 ? `重新发送 (${cooldown}s)` : sending ? '发送中…' : '发送验证码' }}
        </button>
      </template>
      <template v-else-if="step === 'confirm'">
        <button type="button" class="btn btn-secondary" @click="close">{{ dismissText }}</button>
        <button type="button" class="btn btn-primary" :disabled="!code || confirming" @click="doConfirm">{{ confirming ? '验证中…' : '确认验证' }}</button>
      </template>
      <button v-else type="button" class="btn btn-primary" @click="close">完成</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { getEmailVerifyStatus, sendVerifyCode, confirmVerifyCode, type EmailVerifyStatusView } from './api'

const props = withDefaults(defineProps<{ modelValue: boolean; dismissText?: string }>(), { dismissText: '稍后再说' })
const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  verified: []
}>()

const router = useRouter()
const open = computed(() => props.modelValue)

type Step = 'send' | 'confirm' | 'success'
const step = ref<Step>('send')
const status = ref<EmailVerifyStatusView | null>(null)
const code = ref('')
const error = ref('')
const sending = ref(false)
const confirming = ref(false)
const cooldown = ref(0)
const rewardGranted = ref(false)
const rewardPoints = ref(0)
const codeInput = ref<HTMLInputElement | null>(null)

const title = computed(() => {
  if (step.value === 'success') return '验证成功'
  if (status.value?.status === 'invalid') return '邮箱无法接收邮件'
  return '请验证你的邮箱'
})

let cooldownTimer: ReturnType<typeof setInterval> | null = null
function stopCooldown() {
  if (cooldownTimer) clearInterval(cooldownTimer)
  cooldownTimer = null
}
function startCooldown(seconds = 60) {
  stopCooldown()
  cooldown.value = seconds
  cooldownTimer = setInterval(() => {
    if (--cooldown.value <= 0) stopCooldown()
  }, 1000)
}

watch(open, (v) => { if (v) void init() }, { immediate: true })

async function init() {
  step.value = 'send'
  code.value = ''
  error.value = ''
  try {
    status.value = await getEmailVerifyStatus()
    if (status.value.status === 'verified') step.value = 'success'
  } catch {
    // 状态拿不到也允许直接发验证码
  }
}

async function doSendCode() {
  if (sending.value || cooldown.value > 0) return
  sending.value = true
  error.value = ''
  try {
    await sendVerifyCode()
    step.value = 'confirm'
    startCooldown(60)
    void nextTick(() => codeInput.value?.focus())
  } catch (e: unknown) {
    error.value = extractApiErrorMessage(e, '发送失败，请稍后重试')
  } finally {
    sending.value = false
  }
}

async function doConfirm() {
  if (!code.value || confirming.value) return
  confirming.value = true
  error.value = ''
  try {
    const result = await confirmVerifyCode(code.value)
    rewardGranted.value = result.reward_granted
    rewardPoints.value = result.reward_points
    step.value = 'success'
    emit('verified')
  } catch (e: unknown) {
    error.value = extractApiErrorMessage(e, '验证码错误，请重新输入')
  } finally {
    confirming.value = false
  }
}

function close() {
  emit('update:modelValue', false)
}

function goProfile() {
  close()
  void router.push('/profile')
}

onBeforeUnmount(stopCooldown)
</script>
