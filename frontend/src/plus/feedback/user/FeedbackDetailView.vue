<!-- [plus] 用户端反馈详情：时间线 + 补充 -->
<template>
  <AppLayout>
    <div class="space-y-4">
      <router-link to="/feedback" class="inline-flex items-center gap-1 text-sm text-gray-500 hover:text-primary-600 dark:text-dark-400 dark:hover:text-primary-400">
        <Icon name="chevronLeft" size="sm" />
        我的反馈
      </router-link>

      <div v-if="!d" class="card p-16 text-center text-sm text-gray-400">{{ error || '正在加载…' }}</div>

      <div v-else class="grid items-start gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
        <!-- 左：对话 -->
        <section class="card overflow-hidden">
          <header class="card-header flex items-center gap-3">
            <span class="text-xl leading-none">{{ sc?.icon ?? '💬' }}</span>
            <h2 class="min-w-0 flex-1 truncate font-semibold text-gray-900 dark:text-white">{{ sc?.label ?? f.scenario }}</h2>
            <StatusPill :status="f.status" />
          </header>

          <div class="px-6 py-5">
            <Timeline :feedback="f" :messages="d.messages" viewer="user" :scenarios="meta?.scenarios ?? []" />
          </div>

          <footer v-if="isOpen(f.status)" class="card-footer bg-gray-50/60 dark:bg-dark-800/40">
            <p v-if="f.status === 'awaiting'" class="mb-2 text-xs font-medium text-primary-600 dark:text-primary-400">管理员已回复，有补充可以继续留言</p>
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
              <ImagePicker v-model="images" :max="meta?.max_images ?? 0" :max-mb="meta?.max_image_mb ?? 5" />
              <button type="button" class="btn btn-primary" :disabled="(!body.trim() && !images.length) || sending" @click="send">
                {{ sending ? '发送中…' : '发送' }}
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
          <p v-if="f.close_reason" class="px-5 py-4 text-gray-600 dark:text-dark-200">{{ f.close_reason }}</p>
          <nav v-if="d.related.length" class="flex flex-wrap gap-2 px-5 py-4">
            <router-link v-for="r in d.related" :key="r.id" :to="`/feedback/${r.id}`" class="badge badge-gray hover:text-primary-600">
              {{ r.relation === 'prev' ? '上一条' : '后续' }} #{{ r.id }}
            </router-link>
          </nav>
        </aside>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
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
const sending = ref(false)

const f = computed(() => d.value!.feedback)
const sc = computed(() => (d.value ? scenarioOf(d.value.feedback.scenario) : undefined))

async function load() {
  error.value = ''
  try {
    d.value = await userApi.detail(Number(route.params.id))
    unreadBadge(true)
  } catch (e) {
    error.value = extractApiErrorMessage(e, '加载失败')
  }
}

async function send() {
  if ((!body.value.trim() && !images.value.length) || sending.value) return
  sending.value = true
  try {
    await userApi.supplement(f.value.id, body.value, images.value)
    body.value = ''
    images.value = []
    await load()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '发送失败'))
  } finally {
    sending.value = false
  }
}

void loadMeta()
watch(
  () => route.params.id,
  (id) => {
    if (!id) return
    d.value = null
    void load()
  },
  { immediate: true }
)
</script>
