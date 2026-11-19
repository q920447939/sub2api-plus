<!-- [plus] 邮件外壳：品牌层（页头 / 版式 / 页脚品牌区）。合规区（退订链接等）由系统按目的强制追加，不在外壳里。 -->
<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex w-full items-center gap-2">
          <p class="flex-1 text-xs text-gray-500">每封邮件 = 外壳 + 正文 + 合规区。营销邮件的退订链接由系统强制附带，外壳无法去掉。</p>
          <button type="button" class="btn btn-primary" @click="router.push('/admin/outreach/layouts/new')"><Icon name="plus" size="md" class="mr-1" />新建外壳</button>
        </div>
      </template>
      <template #table>
        <DataTable :columns="columns" :data="items" :loading="loading" row-key="id" clickable-rows @row-click="(r: Layout) => router.push(`/admin/outreach/layouts/${r.id}`)">
          <template #cell-name="{ row }">
            <span class="inline-block h-3 w-3 rounded-full align-middle" :style="{ background: row.config.theme_color }" />
            <span class="ml-2 text-sm font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
            <span v-if="row.is_default" class="badge badge-primary ml-2">默认</span>
          </template>
          <template #cell-style="{ row }">
            <span class="text-sm text-gray-600 dark:text-dark-300">{{ HEADER_LABEL[row.config.header as LayoutConfig['header']] }} · {{ row.config.style === 'plain' ? '通栏' : '卡片' }} · {{ row.config.width }}px</span>
          </template>
          <template #cell-used_by="{ value }"><span class="text-sm tabular-nums text-gray-500">{{ value }}</span></template>
          <template #cell-updated_at="{ value }"><span class="text-sm text-gray-500">{{ fmtTime(value) }}</span></template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-3" @click.stop>
              <button type="button" class="text-sm text-primary-600 hover:underline" @click="router.push(`/admin/outreach/layouts/${row.id}`)">编辑</button>
              <button v-if="!row.is_default" type="button" class="text-sm text-gray-500 hover:underline" @click="setDefault(row)">设为默认</button>
              <button v-if="!row.is_default" type="button" class="text-sm text-red-500 hover:underline" @click="target = row">删除</button>
            </div>
          </template>
        </DataTable>
      </template>
    </TablePageLayout>
    <ConfirmDialog :show="!!target" title="删除外壳" :message="`删除「${target?.name ?? ''}」？还被模板或活动使用时不能删除；已生成的批次持有快照，不受影响。`" danger confirm-text="删除" @confirm="doDelete" @cancel="target = null" />
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminApi, fmtTime, type Layout, type LayoutConfig } from '../api'

const router = useRouter()
const appStore = useAppStore()
const HEADER_LABEL: Record<LayoutConfig['header'], string> = { none: '无页头', text: '站点名页头', logo: 'Logo 页头' }
const columns: Column[] = [
  { key: 'name', label: '外壳' },
  { key: 'style', label: '样式' },
  { key: 'used_by', label: '使用中（模板+活动）' },
  { key: 'updated_at', label: '更新时间' },
  { key: 'actions', label: '操作' }
]
const items = ref<Layout[]>([])
const loading = ref(false)
const target = ref<Layout | null>(null)

async function load() {
  loading.value = true
  try {
    items.value = await adminApi.layouts()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '加载失败'))
  } finally {
    loading.value = false
  }
}

async function setDefault(l: Layout) {
  try {
    await adminApi.setDefaultLayout(l.id)
    appStore.showSuccess(`已将「${l.name}」设为默认`)
    load()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '操作失败'))
  }
}

async function doDelete() {
  const l = target.value
  target.value = null
  if (!l) return
  try {
    await adminApi.removeLayout(l.id)
    appStore.showSuccess('已删除')
    load()
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '删除失败'))
  }
}

onMounted(load)
</script>
