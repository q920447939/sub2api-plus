<!-- [plus] 提交反馈面板：类型 → 场景 → 描述 → 补充字段 → 图片。全局只挂一个，由 composer 状态控制 -->
<template>
  <Sheet :open="composer.open" title="反馈" @close="close">
    <div v-if="m" class="space-y-5" data-testid="composer">
      <NoticeCard v-if="m.notice" :source="m.notice" :collapsed-px="96" tone="rounded-2xl bg-primary-50 px-4 py-3 text-primary-900 dark:bg-primary-500/10 dark:text-primary-100" />

      <div class="flex justify-center">
        <Segmented v-model="draft.kind" :options="kindOptions" />
      </div>

      <section ref="scenarioBox" :class="shaking === 'scenario' ? 'fb-shake' : ''">
        <div class="grid grid-cols-2 gap-2 rounded-2xl sm:grid-cols-3" :class="scenarioError ? 'p-1 ring-2 ring-red-400/70' : ''">
          <button
            v-for="s in scenarios"
            :key="s.key"
            type="button"
            class="flex items-center gap-2 rounded-2xl px-3 py-2.5 text-left text-sm transition-all"
            :class="
              s.key === draft.scenarioKey
                ? 'bg-primary-500 font-medium text-white shadow-md shadow-primary-500/25'
                : 'bg-white text-gray-700 ring-1 ring-gray-200/80 hover:ring-gray-300 dark:bg-dark-800 dark:text-dark-100 dark:ring-dark-700'
            "
            :aria-pressed="s.key === draft.scenarioKey"
            @click="draft.scenarioKey = s.key"
          >
            <span class="text-base leading-none">{{ s.icon }}</span>
            <span class="truncate">{{ s.label }}</span>
          </button>
        </div>
        <p v-if="scenarioError" class="input-error-text mt-1.5 px-1" role="alert">{{ scenarioError }}</p>
      </section>

      <section :class="shaking === 'content' ? 'fb-shake' : ''">
        <div
          class="overflow-hidden rounded-2xl bg-white ring-1 focus-within:ring-2 dark:bg-dark-800"
          :class="contentError ? 'ring-red-400 focus-within:ring-red-400' : 'ring-gray-200/80 focus-within:ring-primary-400 dark:ring-dark-700'"
        >
          <textarea
            ref="textarea"
            v-model="draft.content"
            rows="5"
            :maxlength="m.content_max"
            :placeholder="scenario?.hint || (draft.kind === 'issue' ? '遇到了什么情况？' : '说说你的想法')"
            :aria-invalid="!!contentError"
            class="block w-full resize-none border-0 bg-transparent px-4 py-3 text-sm text-gray-900 placeholder:text-gray-400 focus:outline-none focus:ring-0 dark:text-white"
            data-testid="composer-content"
            @keydown.meta.enter.prevent="submit"
            @keydown.ctrl.enter.prevent="submit"
          />
          <div class="flex items-end justify-between gap-3 px-4 pb-3">
            <ImagePicker
              v-model="draft.images"
              :max="m.max_images"
              :max-mb="m.max_image_mb"
              :hint="`最多 ${m.max_images} 张 · 请勿包含 API Key`"
              :late-add="lateAdd"
              paste
              @update:uploading="uploading = $event"
            />
            <span class="shrink-0 text-xs tabular-nums" :class="counterTone" data-testid="composer-counter">
              <template v-if="remaining > 0 && length > 0">还差 {{ remaining }} 字 · </template>{{ length }}/{{ m.content_max }}
            </span>
          </div>
        </div>
        <p v-if="contentError" class="input-error-text mt-1.5 px-1" role="alert">{{ contentError }}</p>
      </section>

      <div v-if="scenario?.fields?.length" class="divide-y divide-gray-100 overflow-hidden rounded-2xl bg-white ring-1 ring-gray-200/80 dark:divide-dark-700 dark:bg-dark-800 dark:ring-dark-700">
        <!-- 下拉框本身是按钮，不能包在 label 里（点标签文字会误触发） -->
        <component
          :is="f.type === 'select' ? 'div' : 'label'"
          v-for="f in scenario.fields"
          :key="f.key"
          class="flex gap-3 px-4 py-2.5 text-sm"
          :class="f.type === 'textarea' ? 'items-start' : 'items-center'"
        >
          <span class="w-20 shrink-0 py-1 text-gray-500 dark:text-dark-300">{{ f.label }}</span>
          <Select
            v-if="f.type === 'select'"
            :model-value="draft.extra[f.key] || null"
            :options="fieldOptions[f.key] ?? []"
            :aria-label="f.label"
            placeholder="选填"
            clearable
            class="fb-select min-w-0 flex-1"
            @update:model-value="(v) => (draft.extra[f.key] = v == null ? '' : String(v))"
          >
            <template #selected="{ option }">
              <span :class="option ? '' : 'text-gray-400 dark:text-dark-400'">{{ option ? option.label : '选填' }}</span>
            </template>
          </Select>
          <textarea v-else-if="f.type === 'textarea'" v-model="draft.extra[f.key]" rows="2" :placeholder="f.placeholder || '选填'" class="fb-field resize-none" />
          <input v-else v-model="draft.extra[f.key]" :type="f.type === 'datetime' ? 'datetime-local' : 'text'" :placeholder="f.placeholder || '选填'" class="fb-field" />
        </component>
      </div>

      <p v-if="composer.prevId" class="text-center text-xs text-gray-400">关联反馈 #{{ composer.prevId }}</p>
    </div>
    <div v-else class="py-16 text-center text-sm text-gray-400">正在加载…</div>

    <template #footer>
      <p v-if="serverError" class="input-error-text mb-2 text-center" role="alert" data-testid="composer-error">{{ serverError }}</p>
      <button type="button" class="btn btn-primary w-full rounded-xl py-2.5" :disabled="submitting || uploading > 0" data-testid="composer-submit" @click="submit">
        {{ submitting ? '提交中…' : uploading > 0 ? '图片上传中…' : '提交' }}
      </button>
    </template>
  </Sheet>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Select from '@/components/common/Select.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { userApi, type Kind } from '../api'
import { composer, draft, loadMeta, meta as m, resetDraft } from '../state'
import { KIND_LABEL } from '../format'
import Sheet from './Sheet.vue'
import Segmented from './Segmented.vue'
import ImagePicker from './ImagePicker.vue'
import NoticeCard from './NoticeCard.vue'

const appStore = useAppStore()
const router = useRouter()

// 草稿（类型、场景、描述、图片、补充字段）放在 state.ts：换页、关掉面板都不丢，只在提交成功后清空
const submitting = ref(false)
const uploading = ref(0) // 还在上传的图片数，传完前不能提交
const tried = ref(false) // 点过提交后才把缺项标红，避免一打开就满屏错误
const serverError = ref('')
const shaking = ref<'' | 'scenario' | 'content'>('')
const textarea = ref<HTMLTextAreaElement>()
const scenarioBox = ref<HTMLElement>()

const kindOptions = (['issue', 'suggestion'] as Kind[]).map((k) => ({ value: k, label: KIND_LABEL[k] }))
// meta 也会返回已停用的场景（用来显示历史反馈的名称），新提交只能选启用的
const scenarios = computed(() => (m.value?.scenarios ?? []).filter((s) => s.kind === draft.kind && s.enabled !== false))
const scenario = computed(() => scenarios.value.find((s) => s.key === draft.scenarioKey))
const fieldOptions = computed<Record<string, { value: string; label: string }[]>>(() =>
  Object.fromEntries((scenario.value?.fields ?? []).map((f) => [f.key, (f.options ?? []).map((o) => ({ value: o, label: o }))]))
)
const length = computed(() => draft.content.trim().length)
const min = computed(() => m.value?.content_min ?? 10)
const remaining = computed(() => Math.max(0, min.value - length.value))

const scenarioError = computed(() => (tried.value && !scenario.value ? `请选择${KIND_LABEL[draft.kind]}的场景` : ''))
const contentError = computed(() => {
  if (!tried.value || remaining.value === 0) return ''
  return length.value === 0 ? `请描述一下${draft.kind === 'issue' ? '遇到的情况' : '你的想法'}` : `至少 ${min.value} 个字，还差 ${remaining.value} 个`
})
const counterTone = computed(() => {
  if (contentError.value) return 'text-red-500'
  if (remaining.value > 0 && length.value > 0) return 'text-amber-500'
  return 'text-gray-400'
})

// 切换类型时，原场景不属于新类型就清掉
watch(
  () => draft.kind,
  () => {
    if (!scenario.value) draft.scenarioKey = ''
  }
)
watch([() => draft.kind, () => draft.scenarioKey, () => draft.content], () => (serverError.value = ''))

// 面板关掉后才传完的图片补进草稿；换成新数组，重新打开的选择框才能感知到
function lateAdd(id: number) {
  if (!draft.images.includes(id)) draft.images = [...draft.images, id]
}

// 提交中不让关：失败原因只显示在面板里，关掉就看不到了
function close() {
  if (!submitting.value) composer.open = false
}

// immediate：面板以打开状态重建时（换页、反馈页带 ?new=1 进来）也要加载配置
watch(
  () => composer.open,
  async (open) => {
    if (!open) return
    serverError.value = ''
    await loadMeta()
    // 草稿里的场景如果已被停用，清掉
    if (m.value && draft.scenarioKey && !scenario.value) draft.scenarioKey = ''
    await nextTick()
    // 只在有鼠标的设备上自动聚焦；手机上弹出的键盘会挡住类型和场景
    if (window.matchMedia?.('(hover: hover) and (pointer: fine)').matches) textarea.value?.focus()
  },
  { immediate: true }
)

function flag(which: 'scenario' | 'content') {
  shaking.value = ''
  void nextTick(() => {
    shaking.value = which
    window.setTimeout(() => (shaking.value = ''), 400)
    if (which === 'scenario') scenarioBox.value?.scrollIntoView?.({ behavior: 'smooth', block: 'center' })
    else textarea.value?.focus()
  })
}

async function submit() {
  if (submitting.value || uploading.value > 0) return
  tried.value = true
  if (!scenario.value) return flag('scenario')
  if (remaining.value > 0) return flag('content')
  submitting.value = true
  serverError.value = ''
  try {
    const fields = Object.fromEntries((scenario.value.fields ?? []).map((f) => [f.key, draft.extra[f.key] ?? '']))
    const id = await userApi.submit({
      kind: draft.kind,
      scenario: scenario.value.key,
      content: draft.content,
      extra: fields,
      attachments: [...draft.images],
      prev_id: composer.prevId,
      version: appStore.siteVersion
    })
    composer.open = false
    composer.submitted++
    resetDraft()
    tried.value = false
    appStore.showSuccess('已收到，感谢你的反馈')
    // 在反馈页里提交就直接打开新反馈；在别的页面提交不打断当前操作
    if (router.currentRoute.value.path.startsWith('/feedback')) void router.push(`/feedback/${id}`)
  } catch (e) {
    // 频率限制、场景停用等，直接显示在提交按钮上方，不只是一闪而过的提示
    serverError.value = extractApiErrorMessage(e, '提交失败，请稍后再试')
    // 面板已经收起（例如提交途中换了页面）时再弹一次，失败不能悄无声息
    if (!composer.open) appStore.showError(serverError.value)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.fb-field {
  @apply w-full flex-1 border-0 bg-transparent p-0 py-1 text-sm text-gray-900 placeholder:text-gray-400 focus:outline-none focus:ring-0 dark:text-white;
}
/* 项目的 Select 嵌在列表式卡片里：去掉边框和底色，与同一卡片里的输入框对齐 */
.fb-select :deep(.select-trigger) {
  @apply rounded-lg border-0 bg-transparent px-0 py-1 dark:bg-transparent;
}
.fb-shake {
  animation: fb-shake 0.36s ease;
}
@keyframes fb-shake {
  20%,
  60% {
    transform: translateX(-4px);
  }
  40%,
  80% {
    transform: translateX(4px);
  }
}
@media (prefers-reduced-motion: reduce) {
  .fb-shake {
    animation: none;
  }
}
</style>
