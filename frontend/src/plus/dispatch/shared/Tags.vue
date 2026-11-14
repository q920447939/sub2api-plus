<!-- [plus] 一行的状态标签：新接入、权重设置（写出哪些分组 / 模型）、模型异常、每个检测项的结果。检测项标签可点开详情 -->
<template>
  <div class="flex min-w-[220px] max-w-[440px] flex-wrap items-start gap-x-2.5 gap-y-2">
    <Tag v-if="isNew" text="新接入" tone="violet" :tip="`${createdAt?.slice(0, 10)} 接入`" />
    <Tag v-if="paused.length" text="暂停" tone="dark" :sub="pausedSub" :tip="tipOf(paused)" />
    <Tag v-if="tuned.length" :text="tuned.length === 1 ? `权重 ${ruleText(tuned[0])}` : '权重已调'" tone="tint" :sub="tunedSub" :tip="tipOf(tuned)" />
    <template v-if="!model">
      <Tag v-for="m in modelRules" :key="m.label" :text="m.label" tone="amber" :sub="someNames(m.models)" :tip="m.tip" />
      <Tag v-if="downModels?.length" text="模型异常" tone="red" :sub="downModels.length === 1 ? downModels[0] : `${downModels.length} 个模型`" :tip="downModels.join('\n')" />
    </template>
    <Tag v-for="p in probes" :key="p.item_id" v-bind="probeTag(p)" clickable @click="emit('probe', p.item_id)" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ProbeTag, Rule } from '../api'
import { ruleText, someNames } from './format'
import Tag from './Tag.vue'

const props = defineProps<{
  rules: Rule[]
  probes: ProbeTag[]
  groups: number
  groupName: (id: number) => string
  model?: string // 模型行：只看这个模型的设置
  isNew?: boolean
  createdAt?: string
  downModels?: string[]
}>()
const emit = defineEmits<{ probe: [itemId: number] }>()

const own = computed(() => props.rules.filter((r) => (r.model || '') === (props.model || '')))
const paused = computed(() => own.value.filter((r) => r.share <= 0))
const tuned = computed(() => own.value.filter((r) => r.share > 0))
const tipOf = (rs: Rule[]) => rs.map((r) => `${props.groupName(r.group_id)}  ${ruleText(r)}`).join('\n')
const subOf = (rs: Rule[], all: boolean) => (props.groups > 1 && !all ? someNames(rs.map((r) => props.groupName(r.group_id))) : '')
const pausedSub = computed(() => subOf(paused.value, paused.value.length >= props.groups))
const tunedSub = computed(() => subOf(tuned.value, false))

// 账号行：单独调过的模型直接写出模型名
const modelRules = computed(() => {
  const mr = props.rules.filter((r) => r.model)
  const out: { label: string; models: string[]; tip: string }[] = []
  for (const [label, pick] of [['模型暂停', (r: Rule) => r.share <= 0], ['模型权重', (r: Rule) => r.share > 0]] as const) {
    const rs = mr.filter(pick)
    if (rs.length) out.push({ label, models: [...new Set(rs.map((r) => r.model))], tip: rs.map((r) => `${r.model}  ${props.groupName(r.group_id)}  ${ruleText(r)}`).join('\n') })
  }
  return out
})

function probeTag(p: ProbeTag): { text: string; sub?: string; tone: 'green' | 'oamber' | 'gray' | 'red' } {
  const sub = p.models?.length ? (props.model ? p.short : someNames(p.models)) : p.short
  switch (p.state) {
    case 'fail':
      return { text: `${p.name}未通过`, sub, tone: 'oamber' }
    case 'pass':
      return { text: `${p.name}通过`, tone: 'green' }
    case 'ignored':
      return { text: `${p.name}已标误报`, sub, tone: 'gray' }
    case 'fault':
      return { text: `${p.name}测不了`, sub: props.model ? p.short : sub, tone: 'red' }
    case 'few':
      return { text: `${p.name}数据不足`, sub: props.model ? p.short : undefined, tone: 'gray' }
    default:
      return { text: `${p.name}未检测`, tone: 'gray' }
  }
}
</script>
