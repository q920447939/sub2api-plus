<!-- [plus] 管理端「反馈管理 · 设置」 -->
<template>
  <AppLayout>
    <div ref="page" class="space-y-6 pb-16">
      <div v-if="!cfg" class="card p-16 text-center text-sm text-gray-400">{{ error || '正在加载…' }}</div>

      <template v-else>
        <div class="grid items-start gap-6 xl:grid-cols-2">
          <div class="space-y-6">
            <section>
              <SectionTitle>基础</SectionTitle>
              <Group>
                <Line title="开启反馈"><Toggle v-model="cfg.enabled" /></Line>
                <Line title="悬浮按钮" desc="用户端页面右侧边缘"><Toggle v-model="cfg.float_button" /></Line>
              </Group>

            </section>
            <section>
              <SectionTitle>公告</SectionTitle>
              <Group>
                <Line title="显示公告" desc="提交面板和「我的反馈」顶部"><Toggle v-model="cfg.notice.enabled" /></Line>
                <div v-if="cfg.notice.enabled" class="p-4">
                  <MarkdownEditor v-model="cfg.notice.markdown" :rows="8" />
                </div>
              </Group>
            </section>

            <section>
              <SectionTitle>积分</SectionTitle>
              <Group>
                <Line title="默认积分" :desc="rewardOn ? '已采纳的建议结案时预填' : '积分模块未启用，暂时不能发放'"><Num v-model="cfg.reward.default" /></Line>
                <Line title="每次加减"><Num v-model="cfg.reward.step" :min="1" /></Line>
              </Group>
            </section>
          </div>
          <div class="space-y-6">
            <section>
              <div class="mb-2 flex items-center gap-3 px-1">
                <SectionTitle class="!mb-0 flex-1">场景</SectionTitle>
                <Segmented v-model="kindTab" :options="kindOptions" />
                <button type="button" class="btn btn-secondary btn-sm" @click="create"><Icon name="plus" size="sm" class="mr-1" />新建</button>
              </div>
              <Group>
                <div v-for="s in tabScenarios" :key="s.key" class="flex items-center gap-3 px-4 py-2.5" :class="s.enabled ? '' : 'opacity-50'">
                  <span class="w-6 text-center text-lg">{{ s.icon }}</span>
                  <button type="button" class="min-w-0 flex-1 text-left" @click="edit(s)">
                    <span class="block truncate text-sm text-gray-900 dark:text-white">{{ s.label }}</span>
                    <span class="block truncate text-xs text-gray-400">{{ s.hint || '—' }}<template v-if="s.fields?.length"> · {{ s.fields.length }} 个补充字段</template></span>
                  </button>
                  <button type="button" class="fb-icon" :disabled="isFirst(s)" title="上移" aria-label="上移" @click="move(s, -1)"><Icon name="arrowUp" size="sm" /></button>
                  <button type="button" class="fb-icon" :disabled="isLast(s)" title="下移" aria-label="下移" @click="move(s, 1)"><Icon name="arrowDown" size="sm" /></button>
                  <button type="button" class="fb-icon" title="编辑" aria-label="编辑" @click="edit(s)"><Icon name="edit" size="sm" /></button>
                  <Toggle v-model="s.enabled" />
                </div>
              </Group>
              <p v-if="scenarioError" class="input-error-text mt-1.5 px-1" data-testid="scenario-error">{{ KIND_LABEL[scenarioError] }}至少启用一个场景</p>
            </section>

            <section>
              <SectionTitle>规则</SectionTitle>
              <Group>
                <Line title="每人每天最多" desc="0 为不限"><Num v-model="cfg.limits.daily_max" suffix="条" /></Line>
                <Line title="提交间隔" desc="0 为不限"><Num v-model="cfg.limits.interval_seconds" suffix="秒" /></Line>
                <Line title="同时处理中最多" desc="0 为不限"><Num v-model="cfg.limits.open_max" suffix="条" /></Line>
                <Line title="每次最多图片" desc="0 为不允许上传"><Num v-model="cfg.attachments.max_count" suffix="张" :max="9" /></Line>
                <Line title="单张图片上限"><Num v-model="cfg.attachments.max_mb" suffix="MB" :min="1" :max="20" /></Line>
                <Line title="回复后自动结案" desc="回复后用户多少天没回应就自动结案，0 表示不自动结案"><Num v-model="cfg.ask_timeout_days" suffix="天" :max="365" /></Line>
              </Group>
            </section>

          </div>
        </div>
      </template>

      <!-- 吸底保存条：页面较长时也能看到未保存提示和保存按钮 -->
      <div
        v-if="cfg"
        class="sticky bottom-3 z-20 ml-auto flex w-fit items-center gap-3 rounded-full border border-white/80 bg-white/90 p-1.5 shadow-lg backdrop-blur-xl dark:border-dark-600/60 dark:bg-dark-900/85"
        :class="dirty ? 'pl-4' : ''"
      >
        <span v-if="dirty" class="text-sm text-amber-600 dark:text-amber-400">有未保存的修改</span>
        <button type="button" class="btn btn-primary" :disabled="saving || !dirty" @click="save">{{ saving ? '保存中…' : '保存' }}</button>
      </div>
    </div>

    <ScenarioDialog :scenario="editing" :is-new="editingNew" @close="editing = null" @save="saveScenario" @remove="removeScenario" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminApi, type AdminScenario as Scenario, type Config, type Kind } from '../api'
import { KIND_LABEL } from '../format'
import { loadMeta } from '../state'
import { loadAdminScenarios } from './shared'
import Segmented from '../components/Segmented.vue'
import Input from '@/components/common/Input.vue'
import MarkdownEditor from '../components/MarkdownEditor.vue'
import ScenarioDialog from './ScenarioDialog.vue'

const appStore = useAppStore()
const cfg = ref<Config | null>(null)
const version = ref(0)
const rewardOn = ref(true)
const error = ref('')
const saving = ref(false)
const snapshot = ref('')
const dirty = computed(() => !!cfg.value && JSON.stringify(cfg.value) !== snapshot.value)

const kindTab = ref<Kind>('issue')
const kindOptions = (['issue', 'suggestion'] as Kind[]).map((k) => ({ value: k, label: KIND_LABEL[k] }))
const tabScenarios = computed(() => (cfg.value?.scenarios ?? []).filter((s) => s.kind === kindTab.value))

/** 开启反馈时，问题和建议都至少要有一个启用的场景（与后端校验一致） */
const scenarioError = computed<Kind | undefined>(() => {
  const c = cfg.value
  if (!c?.enabled) return undefined
  return (['issue', 'suggestion'] as Kind[]).find((k) => !c.scenarios.some((s) => s.kind === k && s.enabled))
})
const page = ref<HTMLElement>()

async function load() {
  try {
    const r = await adminApi.config()
    cfg.value = r.config
    version.value = r.version
    rewardOn.value = r.reward_enabled
    snapshot.value = JSON.stringify(r.config)
  } catch (e) {
    error.value = extractApiErrorMessage(e, '加载失败')
  }
}

async function save() {
  if (!cfg.value) return
  // 有标红的项先定位过去，不提交
  const bad = page.value?.querySelector<HTMLElement>('.input-error, .input-error-text')
  if (bad) {
    bad.scrollIntoView({ block: 'center', behavior: 'smooth' })
    if (bad.matches('input')) bad.focus({ preventScroll: true })
    return
  }
  saving.value = true
  try {
    version.value = await adminApi.saveConfig(cfg.value, version.value)
    snapshot.value = JSON.stringify(cfg.value)
    appStore.showSuccess('已保存')
    void loadMeta(true)
    void loadAdminScenarios(true)
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

// ─── 场景 ──────────────────────────────────────────────────────────────

const editing = ref<Scenario | null>(null)
const editingNew = ref(false)
let editingKey = ''

function edit(s: Scenario) {
  editingKey = s.key
  editingNew.value = false
  editing.value = s
}

function create() {
  editingKey = ''
  editingNew.value = true
  editing.value = { key: '', kind: kindTab.value, label: '', icon: '💬', hint: '', fields: [], enabled: true }
}

function saveScenario(s: Scenario) {
  const list = cfg.value!.scenarios
  if (editingNew.value) {
    if (list.some((x) => x.key === s.key)) {
      appStore.showError('标识已存在')
      return
    }
    list.push(s)
  } else {
    const i = list.findIndex((x) => x.key === editingKey)
    if (i >= 0) list[i] = s
  }
  editing.value = null
}

function removeScenario() {
  // 删除后历史反馈仍保留 key，只是不再显示名称；一般建议用停用代替
  cfg.value!.scenarios = cfg.value!.scenarios.filter((x) => x.key !== editingKey)
  editing.value = null
}

const isFirst = (s: Scenario) => tabScenarios.value[0]?.key === s.key
const isLast = (s: Scenario) => tabScenarios.value[tabScenarios.value.length - 1]?.key === s.key

// 只在同类型内交换顺序
function move(s: Scenario, dir: -1 | 1) {
  const list = cfg.value!.scenarios
  const peers = tabScenarios.value
  const other = peers[peers.findIndex((x) => x.key === s.key) + dir]
  if (!other) return
  const a = list.indexOf(s)
  const b = list.indexOf(other)
  ;[list[a], list[b]] = [list[b], list[a]]
}

// ─── 布局小组件 ──────────────────────────────────────────────────────────

const Group = defineComponent({
  setup(_, { slots }) {
    return () => h('div', { class: 'card divide-y divide-gray-100 overflow-hidden dark:divide-dark-700' }, slots.default?.())
  }
})

const Line = defineComponent({
  props: { title: { type: String, required: true }, desc: { type: String, default: '' } },
  setup(p, { slots }) {
    return () =>
      h('div', { class: 'flex min-h-[60px] items-center gap-4 px-5 py-3' }, [
        h('div', { class: 'flex-1' }, [
          h('div', { class: 'text-sm text-gray-900 dark:text-white' }, p.title),
          p.desc ? h('div', { class: 'text-xs text-gray-400' }, p.desc) : null
        ]),
        slots.default?.()
      ])
  }
})

const SectionTitle = defineComponent({
  setup(_, { slots }) {
    return () => h('h3', { class: 'mb-2 px-1 text-sm font-semibold text-gray-900 dark:text-white' }, slots.default?.())
  }
})

const Num = defineComponent({
  props: {
    modelValue: { type: Number, required: true },
    suffix: { type: String, default: '' },
    min: { type: Number, default: 0 },
    max: { type: Number, default: undefined }
  },
  emits: ['update:modelValue'],
  setup(p, { emit }) {
    const error = computed(() => {
      const v = p.modelValue
      if (v >= p.min && (p.max == null || v <= p.max)) return ''
      return p.max == null ? `不能小于 ${p.min}` : `范围 ${p.min}–${p.max}`
    })
    return () =>
      h('div', { class: 'w-32 shrink-0' }, [
        h(
          Input,
          {
            type: 'number',
            modelValue: p.modelValue,
            error: error.value,
            'onUpdate:modelValue': (v: string) => emit('update:modelValue', Math.max(0, Math.floor(Number(v) || 0)))
          },
          p.suffix ? { suffix: () => h('span', { class: 'text-sm' }, p.suffix) } : undefined
        )
      ])
  }
})

onBeforeRouteLeave(() => (dirty.value ? window.confirm('有未保存的修改，确定离开？') : true))
watch(kindTab, () => (editing.value = null))
onMounted(load)
</script>

<style scoped>
.fb-icon {
  @apply flex h-8 w-8 items-center justify-center rounded-lg text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 disabled:pointer-events-none disabled:opacity-20 dark:hover:bg-dark-700 dark:hover:text-dark-100;
}
</style>
