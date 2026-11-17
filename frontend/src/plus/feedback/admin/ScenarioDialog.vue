<!-- [plus] 场景编辑：图标、名称、提示文字、补充字段。标识自动生成，管理员不用关心 -->
<template>
  <BaseDialog :show="!!draft" :title="isNew ? '新建场景' : '编辑场景'" width="normal" @close="requestClose">
    <div v-if="draft" ref="body" class="space-y-5" data-testid="scenario-dialog">
      <div>
        <label class="input-label mb-1.5 block">名称 <span class="text-red-500">*</span></label>
        <div class="flex items-start gap-2">
          <IconPicker v-model="draft.icon" />
          <div class="flex-1">
            <Input
              ref="labelInput"
              v-model="draft.label"
              placeholder="例如：调用报错"
              :error="labelError"
              data-testid="scenario-label"
              @keydown.enter="onLabelEnter"
            />
          </div>
        </div>
      </div>

      <Input v-model="draft.hint" label="描述框提示" placeholder="用户填写描述时看到的提示文字" />

      <div>
        <div class="mb-2 flex items-center">
          <span class="input-label flex-1">补充字段</span>
          <button type="button" class="btn btn-secondary btn-sm" data-testid="add-field" @click="addField">
            <Icon name="plus" size="sm" class="mr-1" />添加字段
          </button>
        </div>
        <p v-if="!draft.fields?.length" class="rounded-xl border border-dashed border-gray-200 py-5 text-center text-sm text-gray-400 dark:border-dark-600">
          没有补充字段
        </p>
        <div v-else class="space-y-3">
          <div v-for="(f, i) in draft.fields" :key="f.key" class="rounded-xl border border-gray-200 p-3 dark:border-dark-600" data-testid="field-row">
            <div class="flex items-start gap-2">
              <div class="flex-1">
                <Input v-model="f.label" placeholder="字段名称，例如：模型" :error="touched && !f.label.trim() ? '请填写字段名称' : ''" />
              </div>
              <Select v-model="f.type" :options="FIELD_TYPES" class="w-36 shrink-0" />
              <button type="button" class="btn btn-secondary btn-sm h-[42px] text-gray-400 hover:text-red-500" title="删除字段" aria-label="删除字段" @click="draft.fields!.splice(i, 1)">
                <Icon name="trash" size="sm" />
              </button>
            </div>
            <div class="mt-2">
              <template v-if="f.type === 'select'">
                <OptionsInput v-model="f.options" :error="touched && !f.options?.length" />
                <p v-if="touched && !f.options?.length" class="input-error-text mt-1.5">至少添加一个选项</p>
              </template>
              <Input v-else-if="f.type !== 'datetime'" v-model="f.placeholder" placeholder="输入框提示（选填）" />
            </div>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex w-full items-center gap-3">
        <button v-if="!isNew" type="button" class="btn btn-danger" @click="confirmRemove = true">删除</button>
        <span class="flex-1" />
        <button type="button" class="btn btn-secondary" @click="requestClose">取消</button>
        <button type="button" class="btn btn-primary" data-testid="scenario-save" @click="save">完成</button>
      </div>
    </template>
  </BaseDialog>

  <ConfirmDialog
    :show="confirmRemove"
    title="删除场景"
    message="已有反馈将不再显示该场景名称，可改为停用。"
    confirm-text="删除"
    danger
    @confirm="onConfirmRemove"
    @cancel="confirmRemove = false"
  />
  <ConfirmDialog :show="confirmDiscard" title="放弃修改？" message="" confirm-text="放弃" danger @confirm="onConfirmDiscard" @cancel="confirmDiscard = false" />
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Input from '@/components/common/Input.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import type { AdminScenario as Scenario, FieldType } from '../api'
import IconPicker from '../components/IconPicker.vue'
import OptionsInput from '../components/OptionsInput.vue'

const props = defineProps<{ scenario: Scenario | null; isNew: boolean }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'save', s: Scenario): void; (e: 'remove'): void }>()

const FIELD_TYPES: { value: FieldType; label: string }[] = [
  { value: 'text', label: '单行文字' },
  { value: 'textarea', label: '多行文字' },
  { value: 'select', label: '下拉选择' },
  { value: 'datetime', label: '时间' }
]

const draft = ref<Scenario | null>(null)
const touched = ref(false)
const confirmRemove = ref(false)
const confirmDiscard = ref(false)
const body = ref<HTMLElement>()
const labelInput = ref<InstanceType<typeof Input>>()

/** 比较用快照：没有字段和空字段列表视为相同 */
const snap = (d: Scenario | null) => (d ? JSON.stringify({ ...d, fields: d.fields?.length ? d.fields : undefined }) : '')
let initial = ''

watch(
  () => props.scenario,
  (s) => {
    draft.value = s ? JSON.parse(JSON.stringify(s)) : null
    initial = snap(draft.value)
    touched.value = false
    confirmRemove.value = false
    confirmDiscard.value = false
    // BaseDialog 打开后会把焦点放到右上角 ×，等它之后再聚焦到名称
    if (s) setTimeout(() => labelInput.value?.focus())
  },
  { immediate: true }
)

/** 后端要求标识以小写字母开头，只含小写字母、数字、下划线 */
const genKey = (prefix: string) => `${prefix}_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`

const labelError = computed(() => (touched.value && !draft.value?.label.trim() ? '请填写名称' : ''))
const fieldsValid = computed(() =>
  (draft.value?.fields ?? []).every((f) => f.label.trim() && (f.type !== 'select' || (f.options?.length ?? 0) > 0))
)
const valid = computed(() => !!draft.value?.label.trim() && fieldsValid.value)

function addField() {
  draft.value!.fields = [...(draft.value!.fields ?? []), { key: genKey('f'), label: '', type: 'text' }]
}

function save() {
  touched.value = true
  if (!valid.value) {
    void nextTick(() => {
      const el = body.value?.querySelector<HTMLElement>('.input-error')
      ;(el?.matches('input') ? el : el?.querySelector<HTMLElement>('input'))?.focus()
    })
    return
  }
  const d = draft.value!
  if (!d.key) d.key = genKey('s')
  d.label = d.label.trim()
  // 没有字段时不带 fields，和后端返回（omitempty）一致，避免误判为有修改
  const fields = (d.fields ?? []).map((f) => ({ ...f, label: f.label.trim(), options: f.type === 'select' ? f.options : undefined }))
  d.fields = fields.length ? fields : undefined
  emit('save', d)
}

function onLabelEnter(e: KeyboardEvent) {
  if (e.isComposing || e.keyCode === 229) return // 输入法选词时的回车
  save()
}

function requestClose() {
  if (snap(draft.value) !== initial) confirmDiscard.value = true
  else emit('close')
}

function onConfirmDiscard() {
  confirmDiscard.value = false
  emit('close')
}

function onConfirmRemove() {
  confirmRemove.value = false
  emit('remove')
}
</script>
