<!-- [plus] 反馈中心公告：默认展开；内容较长时可以收起（按公告内容记住，换了新公告会重新展开） -->
<template>
  <div class="fb-notice" :class="tone" data-testid="notice">
    <div
      ref="box"
      class="relative overflow-hidden"
      :class="collapsed ? 'fb-notice-collapsed' : ''"
      :style="collapsed ? { maxHeight: `${collapsedPx}px` } : undefined"
    >
      <MarkdownView :source="source" />
    </div>
    <button
      v-if="collapsible"
      type="button"
      class="mt-1 inline-flex items-center gap-0.5 text-xs font-medium text-primary-600 hover:underline dark:text-primary-400"
      :aria-expanded="!collapsed"
      @click="toggle"
    >
      {{ collapsed ? '展开全部' : '收起' }}
      <Icon :name="collapsed ? 'chevronDown' : 'chevronUp'" size="xs" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import MarkdownView from './MarkdownView.vue'

const props = withDefaults(defineProps<{ source: string; tone?: string; collapsedPx?: number }>(), {
  tone: 'card px-5 py-3 text-gray-600 dark:text-dark-200',
  collapsedPx: 120
})

const STORE_KEY = 'plus_feedback_notice_collapsed'

// 简单哈希：公告内容变了就是一份新公告
function hash(s: string): string {
  let h = 0
  for (let i = 0; i < s.length; i++) h = (Math.imul(31, h) + s.charCodeAt(i)) | 0
  return String(h)
}
const noticeId = computed(() => hash(props.source))

const box = ref<HTMLElement>()
const userCollapsed = ref(localStorage.getItem(STORE_KEY) === noticeId.value)
const tall = ref(false) // 内容高度超过折叠高度，才需要「收起」
const collapsible = computed(() => tall.value)
const collapsed = computed(() => userCollapsed.value && tall.value)

function measure() {
  const inner = box.value?.firstElementChild as HTMLElement | undefined
  if (inner) tall.value = inner.scrollHeight > props.collapsedPx + 8
}

function toggle() {
  userCollapsed.value = !collapsed.value
  if (userCollapsed.value) localStorage.setItem(STORE_KEY, noticeId.value)
  else localStorage.removeItem(STORE_KEY)
}

// 图片异步加载完才知道真实高度，用 ResizeObserver 跟踪内容尺寸
let ro: ResizeObserver | undefined
onMounted(() => {
  measure()
  const inner = box.value?.firstElementChild
  if (inner && typeof ResizeObserver !== 'undefined') {
    ro = new ResizeObserver(measure)
    ro.observe(inner)
  }
})
onBeforeUnmount(() => ro?.disconnect())
watch(
  () => props.source,
  () => {
    userCollapsed.value = localStorage.getItem(STORE_KEY) === noticeId.value
    void nextTick(measure)
  }
)
</script>

<style scoped>
/* 用遮罩渐隐而不是盖一层白色渐变，任何底色上都自然 */
.fb-notice-collapsed {
  -webkit-mask-image: linear-gradient(to bottom, #000 calc(100% - 32px), transparent);
  mask-image: linear-gradient(to bottom, #000 calc(100% - 32px), transparent);
}
</style>
