<!-- [plus] 立即体检：对一个账号直连测一轮，不受计划范围和流量限制（适合刚接入、还没放量的账号） -->
<template>
  <BaseDialog :show="!!target" :title="target ? `立即体检 · ${target.name}` : ''" width="normal" @close="emit('close')">
    <div v-if="target" class="space-y-5" data-testid="dispatch-checkup-dialog">
      <div v-if="!target.direct" class="rounded-lg bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">该账号不支持直连检测</div>
      <div>
        <label class="input-label">检测项</label>
        <div class="flex flex-wrap gap-1.5">
          <button v-for="it in activeItems" :key="it.id" type="button" class="rounded-lg border px-3 py-1 text-sm transition-colors" :class="sel.items.includes(it.id) ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'border-gray-200 text-gray-600 hover:border-gray-300 dark:border-dark-600 dark:text-dark-200'" @click="toggle(sel.items, it.id)">{{ it.name }}</button>
        </div>
      </div>
      <div>
        <label class="input-label">模型</label>
        <div class="flex flex-wrap gap-1.5">
          <button v-for="m in models" :key="m" type="button" class="rounded-lg border px-2.5 py-1 font-mono text-xs transition-colors" :class="sel.models.includes(m) ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'border-gray-200 text-gray-600 hover:border-gray-300 dark:border-dark-600 dark:text-dark-200'" @click="toggle(sel.models, m)">{{ m }}</button>
          <input v-model="extra" class="input w-44 py-1 text-xs" placeholder="其他模型，回车添加" @keyup.enter="addModel" />
        </div>
      </div>
      <div v-if="result" class="flex items-center gap-2 rounded-lg bg-green-50 px-3 py-2 text-sm text-green-800 dark:bg-green-900/20 dark:text-green-200">
        <Icon name="checkCircle" size="sm" />
        <span>已开始 {{ result.queued }} 次检测<template v-if="result.skipped?.length">，跳过 {{ result.skipped.length }} 项</template></span>
        <router-link class="ml-auto font-medium hover:underline" :to="{ path: `${DISPATCH_BASE}/runs`, query: { account: target.id, view: 'list' } }">查看结果</router-link>
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">{{ result ? '关闭' : '取消' }}</button>
        <button type="button" class="btn btn-primary" :disabled="busy || !target?.direct || !sel.items.length || !sel.models.length" data-testid="dispatch-checkup-submit" @click="submit">
          {{ busy ? '提交中…' : '开始' }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi } from '../api'
import { DISPATCH_BASE } from '../routes'
import { loadMeta, meta, specOf } from './meta'

export interface CheckupTarget {
  id: number
  name: string
  platform: string
  direct: boolean
}

const props = defineProps<{ target: CheckupTarget | null }>()
const emit = defineEmits<{ close: []; done: [] }>()
const app = useAppStore()
const sel = reactive<{ items: number[]; models: string[] }>({ items: [], models: [] })
const extra = ref('')
const busy = ref(false)
const result = ref<{ queued: number; skipped: string[] | null } | null>(null)
const custom = ref<string[]>([])

const activeItems = computed(() => (meta.value?.items ?? []).filter((i) => i.enabled && specOf(i.type)?.active))
const models = computed(() => {
  const p = props.target?.platform ?? ''
  const fromTraffic = (meta.value?.models[p] ?? []).map((x) => x.model)
  const fromPlans = (meta.value?.plans ?? []).filter((x) => x.platform === p).flatMap((x) => x.models)
  return [...new Set([...fromPlans, ...fromTraffic, ...custom.value])]
})

watch(() => props.target, async (t) => {
  if (!t) return
  result.value = null
  custom.value = []
  await loadMeta()
  // 默认：所有参与判定的主动检测项 + 这个平台计划里的模型（没有计划时取流量最大的那个）
  sel.items = activeItems.value.filter((i) => !i.display_only).map((i) => i.id)
  const planModels = (meta.value?.plans ?? []).filter((x) => x.platform === t.platform).flatMap((x) => x.models)
  sel.models = planModels.length ? [...new Set(planModels)].slice(0, 2) : models.value.slice(0, 1)
}, { immediate: true })

function toggle<T>(arr: T[], v: T) {
  const i = arr.indexOf(v)
  if (i >= 0) arr.splice(i, 1)
  else arr.push(v)
}
function addModel() {
  const m = extra.value.trim()
  if (m) {
    if (!custom.value.includes(m)) custom.value.push(m)
    if (!sel.models.includes(m)) sel.models.push(m)
  }
  extra.value = ''
}
async function submit() {
  if (!props.target) return
  busy.value = true
  try {
    result.value = await dispatchApi.checkup(props.target.id, { item_ids: sel.items, models: sel.models })
    app.showSuccess(`已排队 ${result.value.queued} 次检测`)
    emit('done')
  } catch (e) {
    app.showError(extractApiErrorMessage(e, '体检失败'))
  } finally {
    busy.value = false
  }
}
</script>
