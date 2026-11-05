<!-- [plus] 新增 / 编辑检测任务 -->
<template>
  <BaseDialog :show="open" :title="task ? '编辑检测任务' : '新增检测任务'" width="wide" @close="emit('close')">
    <div class="space-y-4 text-sm">
      <div class="grid gap-4 md:grid-cols-2">
        <label class="block">
          <span class="mb-1 block font-medium">分组</span>
          <select v-model.number="form.group_id" class="input" :disabled="!!task" data-testid="iq-task-group">
            <option :value="0" disabled>请选择分组</option>
            <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}（{{ g.platform }}）</option>
          </select>
          <span v-if="task" class="mt-1 block text-xs text-gray-400">已创建的任务不能改分组</span>
        </label>
        <label class="block">
          <span class="mb-1 block font-medium">检测模型</span>
          <div class="flex gap-2">
            <input v-model.trim="form.model" class="input font-mono" list="iq-models" placeholder="例如 claude-opus-4-7" data-testid="iq-task-model" />
            <button type="button" class="btn btn-secondary btn-sm whitespace-nowrap" :disabled="loadingModels" @click="fetchModels">
              {{ loadingModels ? '读取中…' : '读取列表' }}
            </button>
          </div>
          <datalist id="iq-models"><option v-for="m in models" :key="m" :value="m" /></datalist>
          <span class="mt-1 block text-xs text-gray-400">候选来自该分组网关的 /v1/models，也可以直接填写</span>
        </label>
      </div>

      <div>
        <span class="mb-1 block font-medium">探测 Key</span>
        <label
          class="mb-2 flex cursor-pointer gap-3 rounded-xl border p-3"
          :class="form.key_mode === 'auto' ? 'border-primary-500 bg-primary-50/50 dark:bg-primary-900/20' : 'border-gray-200 dark:border-dark-600'"
        >
          <input v-model="form.key_mode" type="radio" value="auto" class="mt-1" />
          <div>
            <b>自动生成分组专用 Key</b>
            <div class="text-xs text-gray-500">
              名称：<span class="font-mono">{{ choices?.auto_name || '（先选择分组）' }}</span>
              <template v-if="!task">{{ choices?.auto_key_id ? '，已存在，直接复用' : '，保存时创建' }}</template>
            </div>
            <div class="text-xs text-gray-500">
              归属用户：{{ task ? '任务创建人' : '当前登录的管理员' }}，探测消耗从这个用户扣费（正常计费）。
            </div>
          </div>
        </label>
        <label
          class="flex cursor-pointer gap-3 rounded-xl border p-3"
          :class="form.key_mode === 'manual' ? 'border-primary-500 bg-primary-50/50 dark:bg-primary-900/20' : 'border-gray-200 dark:border-dark-600'"
        >
          <input v-model="form.key_mode" type="radio" value="manual" class="mt-1" />
          <div class="flex-1">
            <b>指定已有 Key</b>
            <div class="mb-1.5 text-xs text-gray-500">只列出绑定在该分组上、且处于启用状态的 Key。</div>
            <select v-model.number="form.api_key_id" class="input" :disabled="form.key_mode !== 'manual'">
              <option :value="0" disabled>请选择</option>
              <option v-for="k in choices?.keys ?? []" :key="k.id" :value="k.id">{{ k.name }} · {{ k.masked }} · {{ k.user_email }}</option>
            </select>
          </div>
        </label>
      </div>

      <div class="grid gap-4 md:grid-cols-2">
        <label class="block">
          <span class="mb-1 block font-medium">检测间隔（分钟）</span>
          <input v-model.number="form.interval_minutes" type="number" min="5" class="input" />
        </label>
        <label class="block">
          <span class="mb-1 block font-medium">保留样本数</span>
          <input v-model.number="form.retention" type="number" min="1" max="500" class="input" />
          <span class="mt-1 block text-xs text-gray-400">成功和失败各保留这么多条，超出自动清理</span>
        </label>
        <label class="block">
          <span class="mb-1 block font-medium">请求格式</span>
          <select v-model="form.format" class="input">
            <option value="">自动（按分组平台）</option>
            <option value="anthropic">anthropic · /v1/messages</option>
            <option value="openai_chat">openai_chat · /v1/chat/completions</option>
            <option value="openai_responses">openai_responses · /v1/responses</option>
            <option value="gemini">gemini · streamGenerateContent</option>
          </select>
        </label>
        <label class="block">
          <span class="mb-1 block font-medium">思考强度</span>
          <select v-model="form.thinking" class="input">
            <option value="off">关闭（模型默认）</option>
            <option value="low">低</option>
            <option value="medium">中</option>
            <option value="high">高</option>
          </select>
          <span class="mt-1 block text-xs text-gray-400">打开后输出 Token 和耗时会明显增加</span>
        </label>
      </div>
      <label class="flex items-center gap-3">
        <Toggle v-model="form.enabled" />
        <span>启用定时检测</span>
      </label>
      <div class="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-300">
        按这个配置，每天约检测 {{ dailyRuns }} 次；每次输出通常在 5k~16k Token。
      </div>
      <div v-if="testResult" class="rounded-lg px-3 py-2 text-xs" :class="testResult.ok ? 'bg-green-50 text-green-700' : 'bg-red-50 text-red-600'">
        {{ testResult.ok ? `连接成功，耗时 ${(testResult.latency_ms / 1000).toFixed(1)}s` : '连接失败' }}
        <span v-if="testResult.reply">· 回复：{{ testResult.reply }}</span>
        <span v-if="testResult.message">· {{ testResult.message }}</span>
      </div>
    </div>
    <template #footer>
      <div class="flex w-full items-center gap-2">
        <button type="button" class="btn btn-secondary" :disabled="testing" @click="test">{{ testing ? '测试中…' : '⚡ 测试连接（发一句话）' }}</button>
        <span class="flex-1" />
        <button type="button" class="btn btn-secondary" @click="emit('close')">取消</button>
        <button type="button" class="btn btn-primary" :disabled="saving" data-testid="iq-task-save" @click="save">{{ saving ? '保存中…' : '保存' }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  createTask, getKeyChoices, listModels, testTask, updateTask,
  type GroupInfo, type KeyChoices, type TaskInput, type TaskView, type TestResult
} from './api'

const props = defineProps<{
  open: boolean
  task: TaskView | null
  groups: GroupInfo[]
  presetGroupId?: number
  defaults: { interval: number; retention: number }
}>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'saved'): void }>()
const appStore = useAppStore()

const form = reactive<TaskInput>(blank())
const choices = ref<KeyChoices | null>(null)
const models = ref<string[]>([])
const loadingModels = ref(false)
const testing = ref(false)
const saving = ref(false)
const testResult = ref<TestResult | null>(null)
const dailyRuns = computed(() => (form.interval_minutes > 0 ? Math.floor(1440 / form.interval_minutes) : 0))

function blank(): TaskInput {
  return {
    group_id: 0, model: '', key_mode: 'auto', api_key_id: 0, format: '', thinking: 'off',
    interval_minutes: 30, retention: 48, enabled: true
  }
}

watch(
  () => props.open,
  (open) => {
    if (!open) return
    testResult.value = null
    models.value = []
    const t = props.task
    Object.assign(form, t
      ? {
          group_id: t.group_id, model: t.model, key_mode: t.key_mode, api_key_id: t.api_key_id, format: t.format,
          thinking: t.thinking, interval_minutes: t.interval_minutes, retention: t.retention, enabled: t.enabled
        }
      : { ...blank(), group_id: props.presetGroupId ?? 0, interval_minutes: props.defaults.interval, retention: props.defaults.retention })
    void loadChoices()
  }
)

watch(() => form.group_id, () => {
  if (!props.task) form.api_key_id = 0
  models.value = []
  void loadChoices()
})

async function loadChoices() {
  choices.value = null
  if (!form.group_id) return
  try {
    choices.value = await getKeyChoices(form.group_id)
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '读取 Key 失败'))
  }
}

async function fetchModels() {
  loadingModels.value = true
  try {
    models.value = await listModels({ ...form })
    appStore.showSuccess(`读到 ${models.value.length} 个模型，在输入框里可以选择`)
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '读取模型失败'))
  } finally {
    loadingModels.value = false
  }
}

async function test() {
  testing.value = true
  testResult.value = null
  try {
    testResult.value = await testTask({ ...form })
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '测试失败'))
  } finally {
    testing.value = false
  }
}

async function save() {
  saving.value = true
  try {
    if (props.task) await updateTask(props.task.id, { ...form })
    else await createTask({ ...form })
    appStore.showSuccess('已保存')
    emit('saved')
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '保存失败'))
  } finally {
    saving.value = false
  }
}
</script>
