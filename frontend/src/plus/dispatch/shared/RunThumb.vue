<!-- [plus] 存档题作品缩略图（按需加载作品内容） -->
<template>
  <button type="button" class="w-32 flex-none text-left" @click="emit('open')">
    <HtmlPreview v-if="html" :html="html" thumb />
    <div v-else class="flex items-center justify-center rounded-lg border border-dashed text-xs" :class="run.outcome === 'saved' ? 'border-gray-200 text-gray-400' : 'border-red-200 bg-red-50 text-red-500'" :style="{ aspectRatio: '10 / 7' }">
      {{ run.outcome === 'saved' ? '加载中…' : '生成失败' }}
    </div>
    <div class="mt-0.5 text-[11px] text-gray-500">{{ shortTime(run.t) }}</div>
  </button>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { dispatchApi, type RunBrief } from '../api'
import HtmlPreview from './HtmlPreview.vue'
import { shortTime } from './format'

const props = defineProps<{ run: RunBrief }>()
const emit = defineEmits<{ open: [] }>()
const html = ref('')
onMounted(async () => {
  if (props.run.outcome !== 'saved') return
  try {
    html.value = (await dispatchApi.run(props.run.id)).run.reply
  } catch {
    /* 缩略图失败不影响页面 */
  }
})
</script>
