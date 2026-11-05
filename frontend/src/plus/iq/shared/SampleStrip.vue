<!--
  [plus] 横排样本条（纯展示）：最新在最左，越旧越往右，可横向滚动。
  卡片下方显示什么由页面通过 #meta 插槽决定——用户页只传时间，管理页额外传耗时和账号。
  组件本身不关心身份，也不请求任何数据。
-->
<template>
  <div class="flex gap-3 overflow-x-auto pb-2 pr-3" data-testid="iq-strip">
    <button
      v-for="(item, i) in items"
      :key="item.key"
      type="button"
      class="group w-48 flex-none text-left"
      data-testid="iq-sample"
      @click="emit('open', i)"
    >
      <div class="relative overflow-hidden rounded-lg ring-primary-500 ring-offset-1 transition group-hover:ring-2">
        <PreviewFrame v-if="item.previewUrl" :src="item.previewUrl" thumb />
        <div
          v-else
          class="flex flex-col items-center justify-center gap-1 px-3 text-center text-xs text-red-500"
          :style="{ aspectRatio: '10 / 7', background: 'repeating-linear-gradient(45deg,#fafafa,#fafafa 8px,#f1f5f9 8px,#f1f5f9 16px)' }"
        >
          <span class="font-semibold">✕ 检测失败</span>
          <span class="line-clamp-3 text-gray-500">{{ item.failText }}</span>
        </div>
        <span
          v-if="i === 0"
          class="absolute left-1.5 top-1.5 rounded bg-primary-600 px-1.5 text-[10px] leading-4 text-white"
        >最新</span>
      </div>
      <div class="mt-1.5 flex flex-wrap items-center gap-1.5 text-xs text-gray-600 dark:text-dark-300">
        <slot name="meta" :index="i" />
      </div>
    </button>
    <div v-if="!items.length" class="flex h-32 w-48 flex-none items-center justify-center rounded-lg border border-dashed border-gray-300 text-xs text-gray-400 dark:border-dark-600">
      {{ emptyText }}
    </div>
  </div>
</template>

<script setup lang="ts">
import PreviewFrame from './PreviewFrame.vue'

export interface StripItem {
  key: string | number
  previewUrl?: string
  failText?: string
}

withDefaults(defineProps<{ items: StripItem[]; emptyText?: string }>(), { emptyText: '暂无样本' })
const emit = defineEmits<{ (e: 'open', index: number): void }>()
</script>
