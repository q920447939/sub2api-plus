<!-- [plus] 账号多选：可搜索的下拉加入，已选的以标签展示 -->
<template>
  <div class="space-y-2">
    <PSelect :model-value="null" block searchable :placeholder="placeholder" :options="candidates" @change="add" />
    <div v-if="modelValue.length" class="flex flex-wrap gap-1.5">
      <span v-for="id in modelValue" :key="id" class="inline-flex items-center gap-1 rounded-md bg-gray-100 py-0.5 pl-2 pr-1 text-xs text-gray-700 dark:bg-dark-700 dark:text-dark-100">
        {{ nameOf(id) }}
        <button type="button" class="rounded p-0.5 text-gray-400 hover:bg-gray-200 hover:text-gray-600 dark:hover:bg-dark-600" @click="remove(id)"><Icon name="x" size="xs" /></button>
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { meta } from './meta'
import PSelect from './PSelect.vue'

const props = withDefaults(defineProps<{ modelValue: number[]; platform?: string; placeholder?: string }>(), { placeholder: '选择账号' })
const emit = defineEmits<{ 'update:modelValue': [v: number[]] }>()

const accounts = computed(() => (meta.value?.accounts ?? []).filter((a) => !props.platform || a.platform === props.platform))
const candidates = computed(() => accounts.value.filter((a) => !props.modelValue.includes(a.id)).map((a) => ({ value: a.id, label: a.name })))
const nameOf = (id: number) => meta.value?.accounts.find((a) => a.id === id)?.name ?? `#${id}`
function add(v: unknown) {
  const id = Number(v)
  if (id && !props.modelValue.includes(id)) emit('update:modelValue', [...props.modelValue, id])
}
function remove(id: number) {
  emit('update:modelValue', props.modelValue.filter((x) => x !== id))
}
</script>
