<!-- [plus] 账号多选：就地勾选（可搜索、全选当前结果），不用反复打开下拉。调度已关闭的账号默认收起。 -->
<template>
  <div class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
    <div class="flex items-center gap-2 border-b border-gray-100 px-3 py-2 dark:border-dark-700">
      <Icon name="search" size="sm" class="text-gray-400" />
      <input v-model="kw" type="text" class="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-gray-400" placeholder="搜索账号或分组" />
      <span class="text-xs text-gray-400">已选 {{ modelValue.length }}</span>
      <button v-if="list.length" type="button" class="text-xs text-primary-600 hover:underline" @click="toggleAll">{{ allOn ? '取消全选' : '全选' }}</button>
      <button v-if="modelValue.length" type="button" class="text-xs text-gray-500 hover:underline" @click="emit('update:modelValue', [])">清空</button>
    </div>
    <div class="max-h-56 overflow-y-auto py-1">
      <label v-for="a in list" :key="a.id" class="flex cursor-pointer items-center gap-2.5 px-3 py-1.5 text-sm hover:bg-gray-50 dark:hover:bg-dark-700">
        <input type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" :checked="modelValue.includes(a.id)" @change="toggle(a.id)" />
        <span :class="a.serving ? 'text-gray-800 dark:text-dark-100' : 'text-gray-400'">{{ a.name }}</span>
        <span class="truncate text-xs text-gray-400">{{ groupText(a.group_ids) }}</span>
        <span v-if="!a.serving" class="ml-auto shrink-0 text-xs text-gray-400">调度已关闭</span>
      </label>
      <div v-if="!list.length" class="px-3 py-4 text-center text-sm text-gray-400">没有匹配的账号</div>
    </div>
    <button v-if="offCount && !showOff" type="button" class="w-full border-t border-gray-100 py-1.5 text-xs text-gray-500 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700" @click="showOff = true">
      显示 {{ offCount }} 个调度已关闭的账号
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { meta } from './meta'

const props = defineProps<{ modelValue: number[]; platform?: string }>()
const emit = defineEmits<{ 'update:modelValue': [v: number[]] }>()
const kw = ref('')
const showOff = ref(false)

const groupName = (id: number) => meta.value?.groups.find((g) => g.id === id)?.name ?? ''
const groupText = (ids: number[]) => (ids ?? []).map(groupName).filter(Boolean).join('、')

const all = computed(() => (meta.value?.accounts ?? []).filter((a) => !props.platform || a.platform === props.platform))
const offCount = computed(() => all.value.filter((a) => !a.serving && !props.modelValue.includes(a.id)).length)
const list = computed(() => {
  const k = kw.value.trim().toLowerCase()
  return all.value
    .filter((a) => a.serving || showOff.value || props.modelValue.includes(a.id) || !!k)
    .filter((a) => !k || a.name.toLowerCase().includes(k) || groupText(a.group_ids).toLowerCase().includes(k))
    .sort((a, b) => Number(b.serving) - Number(a.serving) || a.name.localeCompare(b.name))
})
const allOn = computed(() => list.value.length > 0 && list.value.every((a) => props.modelValue.includes(a.id)))

function toggle(id: number) {
  emit('update:modelValue', props.modelValue.includes(id) ? props.modelValue.filter((x) => x !== id) : [...props.modelValue, id])
}
function toggleAll() {
  const ids = list.value.map((a) => a.id)
  emit('update:modelValue', allOn.value ? props.modelValue.filter((x) => !ids.includes(x)) : [...new Set([...props.modelValue, ...ids])])
}
</script>
