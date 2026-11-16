<!-- [plus] 场景编辑：图标、名称、提示文字、补充字段。标识自动生成，管理员不用关心 -->
<template>
  <BaseDialog :show="!!draft" :title="isNew ? '新建场景' : '编辑场景'" width="normal" @close="emit('close')">
    <div v-if="draft" class="space-y-5" data-testid="scenario-dialog">
      <div>
        <label class="input-label mb-1.5 block">名称 <span class="text-red-500">*</span></label>
        <div class="flex items-start gap-2">
          <IconPicker v-model="draft.icon" />
          <div class="flex-1">
            <Input v-model="draft.label" placeholder="例如：调用报错" :error="labelError" data-testid="scenario-label" />
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
                <Input v-model="f.label" placeholder="字段名称，例如：模型" />
              </div>
              <Select v-model="f.type" :options="FIELD_TYPES" class="w-36 shrink-0" />
              <button type="button" class="btn btn-secondary btn-sm h-[42px] text-gray-400 hover:text-red-500" title="删除字段" aria-label="删除字段" @click="draft.fields!.splice(i, 1)">
                <Icon name="trash" size="sm" />
              </button>
            </div>
            <div class="mt-2">
              <OptionsInput v-if="f.type === 'select'" v-model="f.options" />
              <Input v-else-if="f.type !== 'datetime'" v-model="f.placeholder" placeholder="输入框提示（选填）" />
            </div>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex w-full items-center gap-3">
        <button v-if="!isNew" type="button" class="btn btn-danger" @click="emit('remove')">删除</button>
        <span class="flex-1" />
        <button type="button" class="btn btn-secondary" @click="emit('close')">取消</button>
        <button type="button" class="btn btn-primary" :disabled="!valid" data-testid="scenario-save" @click="save">完成</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
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

watch(
  () => props.scenario,
  (s) => {
    draft.value = s ? JSON.parse(JSON.stringify(s)) : null
    touched.value = false
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
  if (!valid.value) return
  const d = draft.value!
  if (!d.key) d.key = genKey('s')
  d.label = d.label.trim()
  d.fields = (d.fields ?? []).map((f) => ({ ...f, label: f.label.trim(), options: f.type === 'select' ? f.options : undefined }))
  emit('save', d)
}
</script>
