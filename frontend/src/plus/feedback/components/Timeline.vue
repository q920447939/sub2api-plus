<!-- [plus] 反馈时间线：原始提交 + 对话 + 系统事件。viewer 决定哪一侧是「我」 -->
<template>
  <ol class="space-y-4">
    <li class="flex" :class="viewer === 'user' ? 'justify-end' : 'justify-start'">
      <Bubble :mine="viewer === 'user'" :time="feedback.created_at" :who="viewer === 'admin' ? '用户' : ''">
        <p class="whitespace-pre-wrap break-words">{{ feedback.content }}</p>
        <dl v-if="extras.length" class="mt-2 space-y-0.5 border-t border-black/5 pt-2 text-xs dark:border-white/10">
          <div v-for="e in extras" :key="e.label" class="flex gap-2">
            <dt class="shrink-0 opacity-60">{{ e.label }}</dt>
            <dd class="whitespace-pre-wrap break-all">{{ e.value }}</dd>
          </div>
        </dl>
        <div v-if="feedback.attachments.length" class="mt-2 flex flex-wrap gap-2">
          <AuthImage v-for="id in feedback.attachments" :key="id" :id="id" :admin="viewer === 'admin'" />
        </div>
      </Bubble>
    </li>

    <li v-for="m in messages" :key="m.id" class="flex" :class="align(m)">
      <p v-if="m.kind === 'event'" class="max-w-md text-center text-xs text-gray-400 dark:text-dark-400">
        <template v-for="(part, i) in eventParts(m)" :key="i">
          <router-link v-if="part.id" :to="linkTo(part.id)" class="text-primary-600 hover:underline dark:text-primary-400">#{{ part.id }}</router-link>
          <span v-else>{{ part.text }}</span>
        </template>
        <span class="ml-1.5 opacity-70">· {{ ago(m.created_at) }}</span>
      </p>
      <Bubble v-else :mine="isMine(m)" :tone="m.kind === 'note' ? 'note' : undefined" :time="m.created_at" :who="who(m)">
        <p v-if="m.body" class="whitespace-pre-wrap break-words">{{ m.body }}</p>
        <div v-if="m.attachments.length" class="mt-2 flex flex-wrap gap-2">
          <AuthImage v-for="id in m.attachments" :key="id" :id="id" :admin="viewer === 'admin'" />
        </div>
      </Bubble>
    </li>
  </ol>
</template>

<script setup lang="ts">
import { computed, defineComponent, h } from 'vue'
import type { Feedback, Message, Scenario } from '../api'
import { KIND_LABEL, SEVERITY_META, STATUS_META, ago, fullTime } from '../format'
import AuthImage from './AuthImage.vue'

const props = defineProps<{ feedback: Feedback; messages: Message[]; viewer: 'user' | 'admin'; scenarios: Scenario[] }>()

const scenario = computed(() => props.scenarios.find((s) => s.key === props.feedback.scenario))

// 补充字段按场景定义的顺序显示；场景已删除时直接显示 key
const extras = computed(() => {
  const extra = props.feedback.extra ?? {}
  const defined = scenario.value?.fields ?? []
  const out = defined.filter((f) => extra[f.key]).map((f) => ({ label: f.label, value: extra[f.key] }))
  for (const [k, v] of Object.entries(extra)) if (!defined.some((f) => f.key === k)) out.push({ label: k, value: v })
  return out
})

const isMine = (m: Message) => (props.viewer === 'user' ? m.author_type === 'user' : m.author_type === 'admin')
const align = (m: Message) => (m.kind === 'event' ? 'justify-center' : isMine(m) ? 'justify-end' : 'justify-start')

function who(m: Message): string {
  if (m.kind === 'note') return '内部备注'
  if (m.kind === 'ask') return props.viewer === 'user' ? '管理员' : '追问'
  if (props.viewer === 'user' && m.author_type === 'admin') return '回复'
  return ''
}

const linkTo = (id: number) => (props.viewer === 'admin' ? `/admin/feedback/${id}` : `/feedback/${id}`)
const scenarioLabel = (key?: string) => props.scenarios.find((s) => s.key === key)?.label ?? key ?? ''

type Part = { text?: string; id?: number }

function eventParts(m: Message): Part[] {
  const e = m.event
  if (!e) return []
  switch (e.type) {
    case 'closed':
      // 主反馈通常是别人提的，用户端没有权限打开，不给链接
      if (e.status === 'duplicate' && e.ref_id) return props.viewer === 'admin' ? [{ text: '已合并到 ' }, { id: e.ref_id }] : [{ text: '与已有反馈重复，已合并处理' }]
      return [{ text: `标记为「${e.status ? STATUS_META[e.status].label : ''}」` }]
    case 'auto_close':
      return [{ text: '超时未补充，已自动结案' }]
    case 'reward':
      return [{ text: `获得 ${e.points} 积分 🎉` }]
    case 'reclass':
      return [{ text: `调整为 ${e.kind ? KIND_LABEL[e.kind] : ''} · ${scenarioLabel(e.scenario)}` }]
    case 'severity':
      return [{ text: e.severity ? `定级为「${SEVERITY_META[e.severity].label}」` : '清除定级' }]
    case 'tags':
      return [{ text: e.tags?.length ? `标签：${e.tags.join('、')}` : '清除标签' }]
    case 'merged_in':
      return [{ id: e.ref_id }, { text: ' 合并到这里' }]
    case 'follow_up':
      return [{ text: '有新的关联反馈 ' }, { id: e.ref_id }]
    default:
      return []
  }
}

// 对话气泡
const Bubble = defineComponent({
  props: { mine: Boolean, tone: { type: String, default: '' }, time: { type: String, required: true }, who: { type: String, default: '' } },
  setup(p, { slots }) {
    return () => {
      const tone =
        p.tone === 'note'
          ? 'bg-amber-50 text-amber-900 ring-1 ring-amber-200 dark:bg-amber-500/10 dark:text-amber-100 dark:ring-amber-500/30'
          : p.mine
            ? 'bg-primary-500 text-white'
            : 'bg-white text-gray-800 ring-1 ring-gray-200/70 dark:bg-dark-800 dark:text-dark-100 dark:ring-dark-700'
      return h('div', { class: ['flex max-w-[min(36rem,85%)] flex-col', p.mine ? 'items-end' : 'items-start'] }, [
        p.who ? h('span', { class: 'mb-1 px-1 text-xs font-medium text-gray-500 dark:text-dark-300' }, p.who) : null,
        h('div', { class: ['rounded-2xl px-4 py-2.5 text-sm shadow-sm', tone, p.mine ? 'rounded-br-md' : 'rounded-bl-md'] }, slots.default?.()),
        h('time', { class: 'mt-1 px-1 text-[11px] text-gray-400 dark:text-dark-400', title: fullTime(p.time) }, ago(p.time))
      ])
    }
  }
})
</script>
