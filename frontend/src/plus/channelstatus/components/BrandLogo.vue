<!-- [plus] 渠道状态的品牌 Logo（彩色）：先按名称里的模型关键词识别（如 openai 平台下的「deepseek」分组显示 DeepSeek），
     识别不出再按平台；都不认识时回退到通用的单色平台图标 -->
<template>
  <span class="inline-flex shrink-0 items-center" :class="{ 'dark:invert': mono }" :data-brand="brand ?? 'none'" data-testid="cs-brand-logo">
    <ModelIcon v-if="brand" :model="brand" :size="size" />
    <PlatformIcon v-else :platform="platform" size="sm" />
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'

const props = withDefaults(defineProps<{ platform: string; name?: string; size?: string }>(), { name: '', size: '16px' })

// 值是 ModelIcon 认得的模型名
const BY_KEYWORD: [RegExp, string][] = [
  [/deepseek/i, 'deepseek'],
  [/grok|xai/i, 'grok'],
  [/gemini/i, 'gemini'],
  [/claude/i, 'claude'],
  [/glm|智谱|zhipu/i, 'glm'],
  [/kimi|moonshot/i, 'kimi'],
  [/qwen|通义/i, 'qwen'],
  [/minimax/i, 'minimax'],
  [/gpt|openai|codex/i, 'gpt']
]
const BY_PLATFORM: Record<string, string> = {
  anthropic: 'claude',
  kiro: 'claude',
  openai: 'gpt',
  gemini: 'gemini',
  antigravity: 'gemini',
  grok: 'grok',
  deepseek: 'deepseek',
  kimi: 'kimi',
  zhipu: 'glm',
  minimax: 'minimax'
}
// 官方 Logo 是黑色的品牌，深色模式下反色
const MONO = new Set(['gpt', 'grok', 'kimi'])

const brand = computed(() => BY_KEYWORD.find(([re]) => re.test(props.name))?.[1] ?? BY_PLATFORM[props.platform] ?? null)
const mono = computed(() => brand.value != null && MONO.has(brand.value))
</script>
