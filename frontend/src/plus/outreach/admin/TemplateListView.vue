<!-- [plus] 邮件模板库：按目的分类的可复用内容。内置模板可编辑不可删除（单发验证邀请等直发使用它）。 -->
<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex w-full flex-wrap items-center gap-2">
          <Select v-model="purpose" :options="purposeOptions" class="w-44" />
          <div class="flex-1" />
          <button type="button" class="btn btn-primary" @click="router.push({ path: '/admin/outreach/templates/new', query: purpose ? { purpose } : {} })">
            <Icon name="plus" size="md" class="mr-1" />新建模板
          </button>
        </div>
      </template>
      <template #table>
        <DataTable :columns="columns" :data="items" :loading="loading" row-key="id" clickable-rows @row-click="(r: Template) => router.push(`/admin/outreach/templates/${r.id}`)">
          <template #cell-name="{ row }">
            <span class="text-sm font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
            <span v-if="row.builtin" class="badge badge-gray ml-2">内置</span>
            <p class="max-w-md truncate text-xs text-gray-500">{{ row.subject }}</p>
          </template>
          <template #cell-purpose="{ row }"><span class="text-sm text-gray-600 dark:text-dark-300">{{ purposeLabel(row.purpose) }}</span></template>
          <template #cell-layout="{ row }"><span class="text-sm text-gray-500">{{ layoutName(row.layout_id) }}</span></template>
          <template #cell-updated_at="{ value }"><span class="text-sm text-gray-500">{{ fmtTime(value) }}</span></template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-3" @click.stop>
              <button type="button" class="text-sm text-primary-600 hover:underline" @click="router.push(`/admin/outreach/templates/${row.id}`)">编辑</button>
              <button type="button" class="text-sm text-gray-500 hover:underline" @click="router.push({ path: '/admin/outreach/templates/new', query: { from: row.id } })">复制</button>
              <button v-if="!row.builtin" type="button" class="text-sm text-red-500 hover:underline" @click="target = row">删除</button>
            </div>
          </template>
          <template #empty><EmptyState title="没有模板" description="新建一个模板，新建活动时可以直接套用" /></template>
        </DataTable>
      </template>
    </TablePageLayout>
    <ConfirmDialog :show="!!target" title="删除模板" :message="`删除「${target?.name ?? ''}」？已经用它创建的活动不受影响（内容已复制到活动里）。`" danger confirm-text="删除" @confirm="doDelete" @cancel="target = null" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Select from '@/components/common/Select.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminApi, fmtTime, useOutreachMeta, type Layout, type Template } from '../api'

const router = useRouter()
const appStore = useAppStore()
const { meta, purposeLabel } = useOutreachMeta()

const columns: Column[] = [
  { key: 'name', label: '模板' },
  { key: 'purpose', label: '目的' },
  { key: 'layout', label: '外壳' },
  { key: 'updated_at', label: '更新时间' },
  { key: 'actions', label: '操作' }
]
const purpose = ref('')
const purposeOptions = computed(() => [{ value: '', label: '全部目的' }, ...(meta.value?.purposes ?? []).map((p) => ({ value: p.key, label: p.label }))])
const items = ref<Template[]>([])
const layouts = ref<Layout[]>([])
const loading = ref(false)
const target = ref<Template | null>(null)
const layoutName = (id: number | null) => (id ? layouts.value.find((l) => l.id === id)?.name ?? '—' : '默认外壳')

async function load() {
  loading.value = true
  try {
    items.value = await adminApi.templates(purpose.value)
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '加载失败'))
  } finally {
    loading.value = false
  }
}

async function doDelete() {
  const t = target.value
  target.value = null
  if (!t) return
  try {
    await adminApi.removeTemplate(t.id)
    appStore.showSuccess('已删除')
    load()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '删除失败'))
  }
}

watch(purpose, load)
onMounted(() => {
  load()
  adminApi.layouts().then((l) => (layouts.value = l)).catch(() => {})
})
</script>
