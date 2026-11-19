<!-- [plus] 邮件预览：后端渲染的 HTML 放进无脚本 iframe -->
<template>
  <section class="card">
    <div class="border-b border-gray-100 px-6 py-3 dark:border-dark-700">
      <p class="text-xs text-gray-500">{{ caption }}</p>
      <p class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ subject || '（主题为空）' }}</p>
    </div>
    <iframe :srcdoc="html" sandbox="" class="w-full rounded-b-xl bg-white" :style="{ height: `${height}px` }" title="邮件预览" />
  </section>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ subject: string; html: string; caption?: string; height?: number }>(), {
  caption: '邮件预览（示例数据）',
  height: 640
})
</script>
