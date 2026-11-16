<!-- [plus] 提交反馈面板：类型 → 场景 → 描述 → 补充字段 → 图片。全局只挂一个，由 composer 状态控制 -->
<template>
  <Sheet :open="composer.open" title="反馈" @close="composer.open = false">
    <div v-if="m" class="space-y-5">
      <NoticeCard v-if="m.notice" :source="m.notice" :collapsed-px="96" tone="rounded-2xl bg-primary-50 px-4 py-3 text-primary-900 dark:bg-primary-500/10 dark:text-primary-100 [&_.fb-notice-fade]:hidden" />

      <div class="flex justify-center">
        <Segmented v-model="kind" :options="kindOptions" />
      </div>

      <div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
        <button
          v-for="s in scenarios"
          :key="s.key"
          type="button"
          class="flex items-center gap-2 rounded-2xl px-3 py-2.5 text-left text-sm transition-all"
          :class="
            s.key === scenarioKey
              ? 'bg-primary-500 font-medium text-white shadow-md shadow-primary-500/25'
              : 'bg-white text-gray-700 ring-1 ring-gray-200/80 hover:ring-gray-300 dark:bg-dark-800 dark:text-dark-100 dark:ring-dark-700'
          "
          @click="scenarioKey = s.key"
        >
          <span class="text-base leading-none">{{ s.icon }}</span>
          <span class="truncate">{{ s.label }}</span>
        </button>
      </div>

      <div class="overflow-hidden rounded-2xl bg-white ring-1 ring-gray-200/80 focus-within:ring-2 focus-within:ring-primary-400 dark:bg-dark-800 dark:ring-dark-700">
        <textarea
          ref="textarea"
          v-model="content"
          rows="5"
          :maxlength="m.content_max"
          :placeholder="scenario?.hint || (kind === 'issue' ? '遇到了什么情况？' : '说说你的想法')"
          class="block w-full resize-none border-0 bg-transparent px-4 py-3 text-sm text-gray-900 placeholder:text-gray-400 focus:outline-none focus:ring-0 dark:text-white"
        />
        <div class="flex items-end justify-between gap-3 px-4 pb-3">
          <ImagePicker v-model="images" :max="m.max_images" :max-mb="m.max_image_mb" paste />
          <span class="shrink-0 text-xs tabular-nums" :class="tooShort ? 'text-gray-300 dark:text-dark-500' : 'text-gray-400'">
            {{ length }}/{{ m.content_max }}
          </span>
        </div>
      </div>

      <div v-if="scenario?.fields?.length" class="divide-y divide-gray-100 overflow-hidden rounded-2xl bg-white ring-1 ring-gray-200/80 dark:divide-dark-700 dark:bg-dark-800 dark:ring-dark-700">
        <label v-for="f in scenario.fields" :key="f.key" class="flex gap-3 px-4 py-2.5 text-sm" :class="f.type === 'textarea' ? 'items-start' : 'items-center'">
          <span class="w-20 shrink-0 py-1 text-gray-500 dark:text-dark-300">{{ f.label }}</span>
          <select v-if="f.type === 'select'" :value="extra[f.key] ?? ''" class="fb-field" :class="extra[f.key] ? '' : '!text-gray-400'" @change="extra[f.key] = ($event.target as HTMLSelectElement).value">
            <option value="">选填</option>
            <option v-for="o in f.options" :key="o" :value="o">{{ o }}</option>
          </select>
          <textarea v-else-if="f.type === 'textarea'" v-model="extra[f.key]" rows="2" :placeholder="f.placeholder || '选填'" class="fb-field resize-none" />
          <input v-else v-model="extra[f.key]" :type="f.type === 'datetime' ? 'datetime-local' : 'text'" :placeholder="f.placeholder || '选填'" class="fb-field" />
        </label>
      </div>

      <p v-if="composer.prevId" class="text-center text-xs text-gray-400">关联反馈 #{{ composer.prevId }}</p>
    </div>
    <div v-else class="py-16 text-center text-sm text-gray-400">正在加载…</div>

    <template #footer>
      <button type="button" class="btn btn-primary w-full rounded-xl py-2.5" :disabled="!canSubmit || submitting" @click="submit">
        {{ submitting ? '提交中…' : '提交' }}
      </button>
    </template>
  </Sheet>
</template>

<script setup lang="ts">
import { computed, nextTick, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { userApi, type Kind } from '../api'
import { composer, loadMeta, meta as m } from '../state'
import { KIND_LABEL } from '../format'
import Sheet from './Sheet.vue'
import Segmented from './Segmented.vue'
import ImagePicker from './ImagePicker.vue'
import NoticeCard from './NoticeCard.vue'

const appStore = useAppStore()
const router = useRouter()

const kind = ref<Kind>('issue')
const scenarioKey = ref('')
const content = ref('')
const images = ref<number[]>([])
const extra = reactive<Record<string, string>>({})
const submitting = ref(false)
const textarea = ref<HTMLTextAreaElement>()

const kindOptions = (['issue', 'suggestion'] as Kind[]).map((k) => ({ value: k, label: KIND_LABEL[k] }))
const scenarios = computed(() => (m.value?.scenarios ?? []).filter((s) => s.kind === kind.value))
const scenario = computed(() => scenarios.value.find((s) => s.key === scenarioKey.value))
const length = computed(() => content.value.trim().length)
const tooShort = computed(() => length.value < (m.value?.content_min ?? 10))
const canSubmit = computed(() => !!scenario.value && !tooShort.value)

// 切换类型时，原场景不属于新类型就清掉
watch(kind, () => {
  if (!scenario.value) scenarioKey.value = ''
})

function reset() {
  content.value = ''
  images.value = []
  for (const k of Object.keys(extra)) delete extra[k]
  kind.value = 'issue'
  scenarioKey.value = ''
}

watch(
  () => composer.open,
  async (open) => {
    if (!open) return
    await loadMeta()
    reset()
    await nextTick()
    textarea.value?.focus()
  }
)

async function submit() {
  if (!canSubmit.value || !scenario.value) return
  submitting.value = true
  try {
    const fields = Object.fromEntries((scenario.value.fields ?? []).map((f) => [f.key, extra[f.key] ?? '']))
    const id = await userApi.submit({
      kind: kind.value,
      scenario: scenario.value.key,
      content: content.value,
      extra: fields,
      attachments: images.value,
      prev_id: composer.prevId,
      version: appStore.siteVersion
    })
    composer.open = false
    composer.submitted++
    appStore.showSuccess('已收到，感谢你的反馈')
    if (router.currentRoute.value.path.startsWith('/feedback')) void router.push(`/feedback/${id}`)
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '提交失败'))
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.fb-field {
  @apply w-full flex-1 border-0 bg-transparent p-0 py-1 text-sm text-gray-900 placeholder:text-gray-400 focus:outline-none focus:ring-0 dark:text-white;
}
</style>
