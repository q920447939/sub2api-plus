<!-- [plus] 用户端反馈详情：时间线 + 补充 -->
<template>
  <AppLayout>
    <div class="space-y-4">
      <router-link to="/feedback" class="inline-flex items-center gap-1 text-sm text-gray-500 hover:text-primary-600 dark:text-dark-400 dark:hover:text-primary-400">
        <Icon name="chevronLeft" size="sm" />
        我的反馈
      </router-link>

      <div v-if="!d" class="card">
        <EmptyState v-if="error" title="加载失败" :description="error">
          <template #action>
            <button type="button" class="btn btn-secondary" data-testid="detail-retry" @click="load">重试</button>
          </template>
        </EmptyState>
        <div v-else class="flex justify-center p-16"><LoadingSpinner /></div>
      </div>

      <div v-else class="grid items-start gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
        <!-- 左：对话 -->
        <section class="card overflow-hidden">
          <header class="card-header flex items-center gap-3">
            <span class="text-xl leading-none">{{ sc?.icon ?? '💬' }}</span>
            <h2 class="min-w-0 flex-1 truncate font-semibold text-gray-900 dark:text-white">{{ sc?.label || KIND_LABEL[f.kind] }}</h2>
            <StatusPill :status="f.status" />
          </header>

          <p v-if="f.close_reason" class="border-b border-gray-100 bg-gray-50/60 px-6 py-3 text-sm text-gray-600 dark:border-dark-700 dark:bg-dark-800/40 dark:text-dark-200" data-testid="close-reason">
            {{ f.close_reason }}
          </p>

          <div class="px-6 py-5">
            <Timeline :feedback="f" :messages="d.messages" viewer="user" :scenarios="meta?.scenarios ?? []" />
          </div>

          <footer v-if="isOpen(f.status)" class="card-footer bg-gray-50/60 dark:bg-dark-800/40">
            <p v-if="f.status === 'awaiting'" class="mb-2 text-xs font-medium text-primary-600 dark:text-primary-400">
              {{ meta?.ask_timeout_days ? `管理员已回复 · ${meta.ask_timeout_days} 天内无补充将自动结束` : '管理员已回复，有补充可以继续留言' }}
            </p>
            <textarea
              v-model="body"
              rows="3"
              maxlength="2000"
              placeholder="补充信息，Ctrl + Enter 发送"
              class="input resize-none"
              @keydown.meta.enter="send"
              @keydown.ctrl.enter="send"
            />
            <div class="mt-3 flex items-end justify-between gap-2">
              <ImagePicker v-model="images" v-model:uploading="uploadingCount" :max="meta?.max_images ?? 0" :max-mb="meta?.max_image_mb ?? 5" paste />
              <button type="button" class="btn btn-primary shrink-0" :disabled="(!body.trim() && !images.length) || sending || uploadingCount > 0" @click="send">
                {{ uploadingCount > 0 ? '图片上传中…' : sending ? '发送中…' : '发送' }}
              </button>
            </div>
          </footer>
          <footer v-else class="card-footer flex items-center justify-between gap-3 text-sm text-gray-500 dark:text-dark-400">
            <span>已结束，不能再补充</span>
            <button type="button" class="btn btn-secondary btn-sm" @click="openComposer(route.fullPath, f.id)">继续反馈</button>
          </footer>
        </section>

        <!-- 右：信息 -->
        <aside class="card divide-y divide-gray-100 text-sm dark:divide-dark-700">
          <dl class="space-y-3 px-5 py-4">
            <div class="flex justify-between gap-3"><dt class="text-gray-500 dark:text-dark-400">编号</dt><dd class="tabular-nums">#{{ f.id }}</dd></div>
            <div class="flex justify-between gap-3"><dt class="text-gray-500 dark:text-dark-400">类型</dt><dd>{{ KIND_LABEL[f.kind] }}</dd></div>
            <div class="flex justify-between gap-3"><dt class="text-gray-500 dark:text-dark-400">提交时间</dt><dd class="tabular-nums">{{ fullTime(f.created_at) }}</dd></div>
            <div v-if="f.reward_points" class="flex justify-between gap-3">
              <dt class="text-gray-500 dark:text-dark-400">获得积分</dt>
              <dd class="font-medium text-purple-600 dark:text-purple-400">+{{ f.reward_points }}</dd>
            </div>
          </dl>
          <nav v-if="d.related.length" class="flex flex-wrap gap-2 px-5 py-4">
            <router-link
              v-for="r in d.related"
              :key="r.id"
              :to="`/feedback/${r.id}`"
              :title="r.summary"
              class="badge badge-gray hover:text-primary-600 dark:hover:text-primary-400"
            >
              {{ RELATION_LABEL[r.relation] ?? '关联' }} #{{ r.id }}
            </router-link>
          </nav>
        </aside>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { userApi, type Feedback, type Message, type Related } from '../api'
import { loadMeta, meta, openComposer, scenarioOf, unreadBadge } from '../state'
import { KIND_LABEL, fullTime, isOpen } from '../format'
import Timeline from '../components/Timeline.vue'
import StatusPill from '../components/StatusPill.vue'
import ImagePicker from '../components/ImagePicker.vue'

const route = useRoute()
const appStore = useAppStore()

const d = ref<{ feedback: Feedback; messages: Message[]; related: Related[] } | null>(null)
const error = ref('')
const body = ref('')
const images = ref<number[]>([])
const uploadingCount = ref(0)
const sending = ref(false)

const RELATION_LABEL: Partial<Record<Related['relation'], string>> = { prev: '上一条', next: '后续' }

const f = computed(() => d.value!.feedback)
const sc = computed(() => (d.value ? scenarioOf(d.value.feedback.scenario) : undefined))

const statusOf = (e: unknown) => (e as { status?: number } | null)?.status

async function load() {
  const id = String(route.params.id)
  error.value = ''
  try {
    const res = await userApi.detail(Number(id))
    if (id !== String(route.params.id)) return // 已切到别的反馈，丢弃旧响应
    d.value = res
    unreadBadge(true)
  } catch (e) {
    if (id !== String(route.params.id)) return
    const msg = extractApiErrorMessage(e, '加载失败')
    // 已有内容时刷新失败只提示，不把页面清空；反馈不存在了才换成错误页
    if (d.value && statusOf(e) !== 404) appStore.showError(msg)
    else {
      d.value = null
      error.value = msg
    }
  }
}

async function send() {
  if ((!body.value.trim() && !images.value.length) || sending.value || uploadingCount.value > 0) return
  sending.value = true
  try {
    await userApi.supplement(f.value.id, body.value, images.value)
    body.value = ''
    images.value = []
    await load()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '发送失败'))
    // 已被结案或删除：刷新后底部换成「已结束 · 继续反馈」，不让用户反复重试
    const st = statusOf(e)
    if (st === 409 || st === 404) await load()
  } finally {
    sending.value = false
  }
}

// 只有补充框还在时才算有草稿（结案后补充框会消失）
const dirty = () =>
  !!d.value && isOpen(d.value.feedback.status) && (!!body.value.trim() || images.value.length > 0 || uploadingCount.value > 0)
const guard = () => !dirty() || window.confirm('补充内容还没发送，确定离开？')
onBeforeRouteLeave(guard)
onBeforeRouteUpdate(guard) // /feedback/:id 之间跳转复用本组件，走 Update 不走 Leave

void loadMeta()
watch(
  () => route.params.id,
  (id) => {
    if (!id) return
    d.value = null
    body.value = ''
    images.value = []
    void load()
  },
  { immediate: true }
)
</script>
