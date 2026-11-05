<!--
  [plus] 用户端「模型画廊」：一个分组一行，最新在最左；一个分组多个模型时上下堆叠。
  只展示成功样本，卡片下只有时间，点开只有预览动画。没有任何管理员信息或开关。
-->
<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl space-y-5 px-4 py-6 md:px-6" data-testid="iq-user-gallery">
      <header>
        <div class="flex items-center gap-2">
          <component :is="IQNavIcon" class="h-6 w-6 text-primary-500" />
          <h1 class="text-xl font-bold text-gray-900 dark:text-white">{{ title }}</h1>
        </div>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">
          同一道题「鹈鹕骑自行车动画」定时发给各分组的模型，可以直观比较模型的能力。最新的在最左边。
        </p>
      </header>

      <div v-if="disabled" class="card p-10 text-center text-gray-500">{{ title }}暂未开放</div>
      <div v-else-if="!gallery" class="card p-10 text-center text-gray-400">{{ error || '正在加载…' }}</div>
      <div v-else-if="!gallery.groups.length" class="card p-10 text-center text-gray-400">暂无可展示的分组</div>

      <section
        v-for="g in gallery?.groups ?? []"
        :key="g.id"
        class="card flex flex-col overflow-hidden p-0 md:flex-row"
        data-testid="iq-user-row"
      >
        <div class="flex-none border-b border-gray-100 bg-gradient-to-b from-white to-gray-50 p-4 dark:border-dark-700 dark:from-dark-800 dark:to-dark-900 md:w-56 md:border-b-0 md:border-r">
          <h3 class="font-semibold text-gray-900 dark:text-white">{{ g.name }}</h3>
          <span class="mt-1.5 inline-block rounded-full px-2 text-xs" :class="platformBadge(g.platform)">{{ g.platform }}</span>
          <div class="mt-2 text-xs text-gray-500 dark:text-dark-300">
            模型：<span class="font-mono">{{ g.tasks.map((t) => t.model).join('、') }}</span>
          </div>
        </div>
        <div class="min-w-0 flex-1 space-y-3 py-3 pl-3">
          <div v-for="t in g.tasks" :key="t.model">
            <span
              v-if="g.tasks.length > 1"
              class="mb-1.5 inline-block rounded-full border border-primary-200 bg-primary-50 px-2 text-xs text-primary-700 dark:border-primary-800 dark:bg-primary-900/30 dark:text-primary-300"
            >{{ t.model }}</span>
            <SampleStrip :items="t.samples.map((s) => ({ key: s.id, previewUrl: s.preview_url }))" @open="(i) => open(g, t, i)">
              <template #meta="{ index }">
                <span class="font-medium">{{ shortTime(t.samples[index].created_at) }}</span>
              </template>
            </SampleStrip>
          </div>
        </div>
      </section>
    </div>

    <BaseDialog :show="!!current" :title="current ? `${current.group.name} · ${current.task.model}` : ''" width="wide" @close="current = null">
      <template v-if="current">
        <div class="mb-3 flex items-center gap-2 text-sm text-gray-500">
          <span>{{ shortTime(current.task.samples[current.index].created_at) }}</span>
          <span class="flex-1" />
          <button type="button" class="btn btn-secondary btn-sm" :disabled="current.index === 0" @click="current.index--">← 较新</button>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="current.index >= current.task.samples.length - 1" @click="current.index++">较旧 →</button>
        </div>
        <PreviewFrame :key="current.task.samples[current.index].id" :src="current.task.samples[current.index].preview_url" class="rounded-lg border border-gray-200 dark:border-dark-700" />
        <p class="mt-2 text-xs text-gray-400">动画由模型生成，在隔离沙箱中运行。</p>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import PreviewFrame from '../shared/PreviewFrame.vue'
import SampleStrip from '../shared/SampleStrip.vue'
import { platformBadge, shortTime } from '../shared/format'
import { IQNavIcon } from '../navIcon'
import { iqMenuName } from './navFlag'
import { getGallery, type UserGallery, type UserGroup, type UserTask } from './api'

const gallery = ref<UserGallery | null>(null)
const disabled = ref(false)
const error = ref('')
const current = ref<{ group: UserGroup; task: UserTask; index: number } | null>(null)
const title = computed(() => iqMenuName.value)

function open(group: UserGroup, task: UserTask, index: number) {
  current.value = { group, task, index }
}

onMounted(async () => {
  try {
    gallery.value = await getGallery()
  } catch (e: unknown) {
    const status = (e as { status?: number; response?: { status?: number } })?.status ?? (e as { response?: { status?: number } })?.response?.status
    if (status === 404) disabled.value = true
    else error.value = extractApiErrorMessage(e, '加载失败')
  }
})
</script>
