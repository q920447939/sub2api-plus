<!-- [plus] 调度设置：档位（总开关）、偏好、系数、护栏、提醒 -->
<template>
  <div v-if="cfg" class="max-w-3xl space-y-5" data-testid="dispatch-strategy">
    <section class="card p-5">
      <h3 class="mb-3 text-sm font-medium text-gray-900 dark:text-white">档位</h3>
      <div class="inline-flex rounded-xl bg-gray-100 p-1 dark:bg-dark-700">
        <button v-for="(n, k) in MODE_TEXT" :key="k" type="button" class="rounded-lg px-4 py-1.5 text-sm transition-colors" :class="cfg.mode === k ? 'bg-white font-medium text-gray-900 shadow-sm dark:bg-dark-900 dark:text-white' : 'text-gray-500'" :data-testid="`dispatch-mode-${k}`" @click="cfg.mode = k">{{ n }}</button>
      </div>
      <p class="input-hint">{{ MODE_DESC[cfg.mode] }}</p>
    </section>

    <section class="card divide-y divide-gray-100 dark:divide-dark-700">
      <h3 class="px-5 pb-2 pt-4 text-sm font-medium text-gray-900 dark:text-white">调度建议</h3>
      <Row label="异常时份额"><span>×</span><N v-model="cfg.bad_factor" step="0.05" /></Row>
      <Row label="可疑时份额"><span>×</span><N v-model="cfg.warn_factor" step="0.05" /></Row>
      <Row label="挪出的流量优先给">
        <div class="inline-flex rounded-lg bg-gray-100 p-0.5 dark:bg-dark-700">
          <button v-for="o in PREFS" :key="o.k" type="button" class="rounded-md px-3 py-1 text-sm" :class="cfg.pref === o.k ? 'bg-white font-medium text-gray-900 shadow-sm dark:bg-dark-900 dark:text-white' : 'text-gray-500'" @click="cfg.pref = o.k">{{ o.n }}</button>
        </div>
      </Row>
      <Row label="被降账号至少保留"><N v-model="cfg.min_share" /><span>%</span></Row>
      <Row label="每小时最多挪走"><N v-model="cfg.max_move" /><span>%</span></Row>
    </section>

    <section class="card divide-y divide-gray-100 dark:divide-dark-700">
      <h3 class="px-5 pb-2 pt-4 text-sm font-medium text-gray-900 dark:text-white">检测与提醒</h3>
      <Row label="同时检测数上限"><N v-model="cfg.concurrency" /></Row>
      <Row label="未覆盖模型提醒阈值"><span>流量占比 ≥</span><N v-model="cfg.gap_share" /><span>%</span></Row>
      <Row label="连续请求失败提醒"><N v-model="cfg.fault_runs" /><span>次</span></Row>
      <Row label="依据消失后提示恢复"><N v-model="cfg.restore_hint" /><span>小时</span></Row>
    </section>

    <div class="flex justify-end"><button type="button" class="btn btn-primary" :disabled="saving" data-testid="dispatch-strategy-save" @click="save">保存</button></div>
  </div>
</template>

<script setup lang="ts">
import { defineComponent, h, ref } from 'vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { dispatchApi, type Config } from '../api'
import { MODE_DESC, MODE_TEXT } from '../shared/format'
import { loadMeta } from '../shared/meta'
import { refreshMode } from '../shared/mode'

const Row = defineComponent({
  props: { label: { type: String, required: true } },
  setup(p, { slots }) {
    return () => h('div', { class: 'flex items-center justify-between gap-4 px-5 py-3 text-sm' }, [
      h('span', { class: 'text-gray-700 dark:text-dark-100' }, p.label),
      h('span', { class: 'flex items-center gap-1.5 text-gray-500' }, slots.default?.())
    ])
  }
})

const N = defineComponent({
  props: { modelValue: { type: Number, default: 0 }, step: { type: String, default: '1' } },
  emits: ['update:modelValue'],
  setup(p, { emit }) {
    return () => h('input', { type: 'number', step: p.step, class: 'input w-20 px-2 py-1 text-center', value: p.modelValue,
      onChange: (e: Event) => emit('update:modelValue', Number((e.target as HTMLInputElement).value)) })
  }
})

const PREFS = [
  { k: 'quality' as const, n: '质量优先' },
  { k: 'cost' as const, n: '成本优先' }
]
const app = useAppStore()
const cfg = ref<Config | null>(null)
const version = ref(0)
const saving = ref(false)

async function load() {
  const r = await dispatchApi.getConfig()
  cfg.value = r.config
  version.value = r.version
}
void load()

async function save() {
  if (!cfg.value) return
  saving.value = true
  try {
    const r = await dispatchApi.saveConfig(cfg.value, version.value)
    version.value = r.version
    app.showSuccess('已保存')
    await refreshMode()
    await loadMeta(true)
  } catch (e) {
    app.showError(extractApiErrorMessage(e, '保存失败'))
    await load()
  } finally {
    saving.value = false
  }
}
</script>
