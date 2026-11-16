<!-- [plus] 管理端反馈详情：左侧时间线 + 回复，右侧分类与上下文 -->
<template>
  <AppLayout>
    <div class="space-y-4">
      <router-link :to="backTo" class="inline-flex items-center gap-1 text-sm text-gray-500 hover:text-primary-600 dark:text-dark-400 dark:hover:text-primary-400">
        <Icon name="chevronLeft" size="sm" />
        反馈处理
      </router-link>

      <div v-if="!d" class="card p-16 text-center text-sm text-gray-400">{{ error || '正在加载…' }}</div>

      <div v-else class="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
        <!-- 左：对话 -->
        <main class="card min-w-0 overflow-hidden">
          <header class="card-header flex items-center gap-3">
            <span class="text-xl leading-none">{{ scenarioIcon(f.scenario) }}</span>
            <div class="min-w-0 flex-1">
              <h2 class="flex items-center gap-2 truncate font-semibold text-gray-900 dark:text-white">
                {{ KIND_LABEL[f.kind] }} · {{ scenarioLabel(f.scenario) }}
                <span v-if="d.impact > 1" class="badge badge-primary" title="影响人数">×{{ d.impact }}</span>
              </h2>
              <p class="mt-0.5 truncate text-xs text-gray-400">
                #{{ f.id }} · {{ fullTime(f.created_at) }}
                <template v-if="f.kind_user !== f.kind || f.scenario_user !== f.scenario">
                  · 用户原选 {{ KIND_LABEL[f.kind_user ?? f.kind] }} / {{ scenarioLabel(f.scenario_user ?? f.scenario) }}
                </template>
              </p>
            </div>
            <StatusPill :status="f.status" />
            <button v-if="isOpen(f.status)" type="button" class="btn btn-primary btn-sm" @click="closing = true">结案</button>
          </header>

          <p v-if="f.close_reason" class="border-b border-gray-100 bg-gray-50/60 px-6 py-3 text-sm text-gray-600 dark:border-dark-700 dark:bg-dark-800/40 dark:text-dark-200">
            {{ f.close_reason }}
          </p>

          <div class="px-6 py-5">
            <Timeline :feedback="f" :messages="d.messages" viewer="admin" :scenarios="allScenarios" />
          </div>

          <!-- 输入区：结案后只能写内部备注 -->
          <footer class="card-footer" :class="mode === 'note' ? 'bg-amber-50/60 dark:bg-amber-500/5' : 'bg-gray-50/60 dark:bg-dark-800/40'">
            <div class="mb-3">
              <Segmented v-model="mode" :options="modeOptions" />
            </div>
            <textarea
              v-model="body"
              rows="3"
              maxlength="2000"
              :placeholder="placeholder"
              class="input resize-none"
              @keydown.meta.enter="send"
              @keydown.ctrl.enter="send"
            />
            <div class="mt-3 flex items-end justify-between gap-2">
              <ImagePicker v-model="images" :max="attachmentRule.max_count" :max-mb="attachmentRule.max_mb" admin />
              <button
                type="button"
                class="btn"
                :class="mode === 'note' ? 'bg-amber-500 text-white hover:bg-amber-600' : 'btn-primary'"
                :disabled="(!body.trim() && !images.length) || sending"
                @click="send"
              >
                {{ sending ? '发送中…' : mode === 'note' ? '保存备注' : '发送回复' }}
              </button>
            </div>
          </footer>
        </main>

        <!-- 右：分类与上下文 -->
        <aside class="space-y-4">
          <Card title="分类">
            <div class="py-3">
              <div class="mb-2 text-xs text-gray-400">场景</div>
              <Select :model-value="f.scenario" :options="scenarioOptions" searchable @update:model-value="(v) => reclass(String(v))" />
            </div>
            <div class="py-3">
              <div class="mb-2 text-xs text-gray-400">严重等级</div>
              <div class="grid grid-cols-4 gap-1.5">
                <button
                  v-for="s in SEVERITIES"
                  :key="s"
                  type="button"
                  class="flex items-center justify-center gap-1 rounded-lg border py-1.5 text-xs transition-colors"
                  :class="f.severity === s ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'border-gray-200 text-gray-600 hover:border-gray-300 dark:border-dark-600 dark:text-dark-200'"
                  @click="update({ severity: f.severity === s ? '' : s })"
                >
                  <span class="h-1.5 w-1.5 rounded-full" :class="SEVERITY_META[s].dot" />
                  {{ SEVERITY_META[s].label }}
                </button>
              </div>
            </div>
            <div class="py-3">
              <div class="mb-2 text-xs text-gray-400">标签</div>
              <div v-if="f.tags?.length" class="mb-2 flex flex-wrap gap-1.5">
                <span v-for="t in f.tags" :key="t" class="badge badge-gray gap-1">
                  {{ t }}
                  <button type="button" class="text-gray-400 hover:text-red-500" :aria-label="`移除 ${t}`" @click="update({ tags: (f.tags ?? []).filter((x) => x !== t) })">×</button>
                </span>
              </div>
              <Select
                :model-value="null"
                :options="tagChoices"
                placeholder="添加标签"
                searchable
                creatable
                creatable-prefix="新标签："
                search-placeholder="搜索或输入新标签"
                data-testid="tag-select"
                @update:model-value="(v) => addTag(String(v ?? ''))"
              />
            </div>
          </Card>

          <Card v-if="d.user" title="用户">
            <router-link :to="{ path: '/admin/users', query: { search: d.user.email } }" class="block py-2 text-sm font-medium text-gray-900 hover:text-primary-600 dark:text-white">
              {{ d.user.email }}
            </router-link>
            <Row label="余额">${{ d.user.balance.toFixed(2) }}</Row>
            <Row v-if="d.user.subscriptions.length" label="订阅">{{ d.user.subscriptions.join('、') }}</Row>
            <Row label="注册">{{ fullTime(d.user.registered_at).split(' ')[0] }}</Row>
            <Row label="反馈">
              <router-link :to="{ path: '/admin/feedback', query: { status: '', user: d.user.email } }" class="text-primary-600 hover:underline dark:text-primary-400">
                {{ d.user.feedbacks }} 条 · 有效 {{ d.user.accepted }}
              </router-link>
            </Row>
            <Row v-if="d.user.reward_total" label="积分">{{ d.user.reward_total }}</Row>
          </Card>

          <Card :title="`错误请求 ${d.errors.length || ''}`" collapsible :default-open="d.errors.length > 0 && f.kind === 'issue'">
            <p v-if="!d.errors.length" class="py-2 text-xs text-gray-400">提交前 24 小时内没有错误请求</p>
            <details v-for="e in d.errors" :key="e.id" class="group border-b border-gray-100 py-2 last:border-0 dark:border-dark-700">
              <summary class="flex cursor-pointer list-none items-center gap-2 text-xs">
                <span class="rounded bg-red-50 px-1.5 font-mono text-red-600 dark:bg-red-500/10 dark:text-red-300">{{ e.status_code || '—' }}</span>
                <span class="min-w-0 flex-1 truncate text-gray-700 dark:text-dark-200">{{ e.model || e.path }}</span>
                <span class="shrink-0 text-gray-400">{{ shortTime(e.created_at) }}</span>
              </summary>
              <div class="mt-1.5 space-y-0.5 text-xs text-gray-500 dark:text-dark-300">
                <p v-if="e.group">{{ e.platform }} / {{ e.group }}</p>
                <p class="break-all font-mono">{{ e.message }}</p>
              </div>
            </details>
          </Card>

          <Card title="提交环境" collapsible>
            <Row v-if="describeUA(ctx.user_agent)" label="浏览器"><span :title="ctx.user_agent">{{ describeUA(ctx.user_agent) }}</span></Row>
            <Row v-if="ctx.version" label="站点版本">{{ ctx.version }}</Row>
            <Row v-if="ctx.ip" label="IP 地址"><span class="tabular-nums">{{ ctx.ip }}</span></Row>
            <Row v-if="ctx.account" label="当时余额">${{ ctx.account.balance.toFixed(2) }}</Row>
            <p v-if="!ctx.user_agent && !ctx.version && !ctx.ip && !ctx.account" class="py-2 text-xs text-gray-400">没有记录</p>
          </Card>

          <Card v-if="d.related.length" title="关联">
            <router-link v-for="r in d.related" :key="r.id" :to="`/admin/feedback/${r.id}`" class="flex items-center gap-2 py-2 text-sm hover:text-primary-600">
              <span class="shrink-0 text-xs text-gray-400">{{ RELATION[r.relation] }}</span>
              <span class="min-w-0 flex-1 truncate">#{{ r.id }} {{ r.summary }}</span>
              <StatusPill :status="r.status" />
            </router-link>
          </Card>
        </aside>
      </div>
    </div>

    <CloseDialog
      v-if="d"
      ref="closeDialog"
      :open="closing"
      :kind="f.kind"
      :reward-enabled="rewardEnabled"
      :reward-default="rewardRule.default"
      :step="rewardRule.step"
      @close="closing = false"
      @confirm="close"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminApi, type AdminDetail, type CloseRequest, type Related, type UpdateInput } from '../api'
import { KIND_LABEL, SEVERITIES, SEVERITY_META, describeUA, fullTime, isOpen } from '../format'
import { pendingBadge } from '../state'
import { allScenarios, attachmentRule, loadAdminScenarios, rewardEnabled, rewardRule, scenarioIcon, scenarioLabel } from './shared'
import Timeline from '../components/Timeline.vue'
import StatusPill from '../components/StatusPill.vue'
import Segmented from '../components/Segmented.vue'
import ImagePicker from '../components/ImagePicker.vue'
import CloseDialog from './CloseDialog.vue'

const route = useRoute()
const appStore = useAppStore()

const d = ref<AdminDetail | null>(null)
const error = ref('')
const f = computed(() => d.value!.feedback)
const ctx = computed(() => d.value?.feedback.context ?? {})
const backTo = computed(() => (window.history.state?.back?.startsWith('/admin/feedback') ? window.history.state.back : '/admin/feedback'))

// 场景下拉：同时列出问题和建议的场景；已结案时不能跨类型
const scenarioOptions = computed(() =>
  allScenarios.value.map((s) => ({
    value: s.key,
    label: `${KIND_LABEL[s.kind]} · ${s.icon} ${s.label}`,
    disabled: !!d.value && s.kind !== d.value.feedback.kind && !isOpen(d.value.feedback.status)
  }))
)

const RELATION: Record<Related['relation'], string> = { prev: '上一条', next: '后续', merged_in: '合并进来', merged_into: '合并到' }

type Mode = 'reply' | 'note'
const mode = ref<Mode>('reply')
const modeOptions = computed(() =>
  d.value && !isOpen(d.value.feedback.status)
    ? [{ value: 'note' as Mode, label: '内部备注' }]
    : [
        { value: 'reply' as Mode, label: '回复用户' },
        { value: 'note' as Mode, label: '内部备注' }
      ]
)
const placeholder = computed(() => ({ reply: '回复后状态变为「已回复」，用户补充后回到「待处理」', note: '只有管理员能看到' })[mode.value])
const body = ref('')
const images = ref<number[]>([])
const sending = ref(false)
const closing = ref(false)
const closeDialog = ref<InstanceType<typeof CloseDialog>>()
const tagOptions = ref<string[]>([])

async function load() {
  error.value = ''
  try {
    const r = await adminApi.detail(Number(route.params.id))
    d.value = r.detail
    rewardEnabled.value = r.reward_enabled
    if (!isOpen(r.detail.feedback.status)) mode.value = 'note'
  } catch (e) {
    error.value = extractApiErrorMessage(e, '加载失败')
  }
}

async function send() {
  if ((!body.value.trim() && !images.value.length) || sending.value) return
  sending.value = true
  try {
    await adminApi.message(f.value.id, mode.value, body.value, images.value)
    body.value = ''
    images.value = []
    await load()
    pendingBadge(true)
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '发送失败'))
  } finally {
    sending.value = false
  }
}

async function update(input: UpdateInput) {
  try {
    await adminApi.update(f.value.id, input)
    await load()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '保存失败'))
  }
}

// 场景下拉同时决定类型：选了另一类型下的场景就是修正类型
function reclass(key: string) {
  const s = allScenarios.value.find((x) => x.key === key)
  if (s) void update({ kind: s.kind, scenario: s.key })
}

// 已有标签里排除当前这条已经打上的
const tagChoices = computed(() => tagOptions.value.filter((t) => !(d.value?.feedback.tags ?? []).includes(t)).map((t) => ({ value: t, label: t })))

function addTag(t: string) {
  t = t.trim()
  if (!t || (f.value.tags ?? []).includes(t)) return
  if (!tagOptions.value.includes(t)) tagOptions.value.push(t)
  void update({ tags: [...(f.value.tags ?? []), t] })
}

async function close(req: CloseRequest) {
  try {
    await adminApi.close(f.value.id, req)
    closing.value = false
    appStore.showSuccess('已结案')
    pendingBadge(true)
    await load()
  } catch (e) {
    closeDialog.value?.done()
    appStore.showError(extractApiErrorMessage(e, '结案失败'))
  }
}

const shortTime = (iso: string) => new Date(iso).toLocaleString('zh-CN', { hour12: false, month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })

// 右侧卡片（分组列表样式）
const Card = defineComponent({
  props: { title: { type: String, required: true }, collapsible: Boolean, defaultOpen: { type: Boolean, default: false } },
  setup(p, { slots }) {
    const open = ref(!p.collapsible || p.defaultOpen)
    watch(
      () => p.defaultOpen,
      (v) => p.collapsible && (open.value = v)
    )
    return () =>
      h('section', { class: 'card px-5 py-2' }, [
        h(
          p.collapsible ? 'button' : 'div',
          {
            type: p.collapsible ? 'button' : undefined,
            class: 'flex w-full items-center py-2 text-sm font-semibold text-gray-900 dark:text-white',
            onClick: p.collapsible ? () => (open.value = !open.value) : undefined
          },
          [
            h('span', { class: 'flex-1 text-left' }, p.title),
            p.collapsible ? h('span', { class: ['text-gray-400 transition-transform', open.value ? 'rotate-90' : ''] }, '›') : null
          ]
        ),
        open.value ? h('div', { class: 'divide-y divide-gray-100 dark:divide-dark-700' }, slots.default?.()) : null
      ])
  }
})

const Row = defineComponent({
  props: { label: { type: String, required: true } },
  setup(p, { slots }) {
    return () =>
      h('div', { class: 'flex items-center gap-3 py-2 text-sm' }, [
        h('span', { class: 'w-20 shrink-0 text-gray-500 dark:text-dark-400' }, p.label),
        h('div', { class: 'min-w-0 flex-1 truncate text-right text-gray-800 dark:text-dark-100' }, slots.default?.())
      ])
  }
})

void loadAdminScenarios()
adminApi.tags().then((t) => (tagOptions.value = t)).catch(() => {})
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
