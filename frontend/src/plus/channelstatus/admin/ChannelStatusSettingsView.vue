<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-5 pb-24" data-testid="cs-admin">
      <header class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 class="text-xl font-bold text-gray-900 dark:text-white">渠道状态设置</h1>
          <p class="text-sm text-gray-400">配置用户端「渠道状态」页面的展示内容、健康判定规则与兜底探测</p>
        </div>
        <router-link :to="CHANNEL_STATUS_PATH" target="_blank" class="btn btn-secondary btn-sm">预览用户端 ↗</router-link>
      </header>

      <nav class="flex gap-1 border-b border-gray-200 dark:border-dark-700">
        <button
          v-for="t in TABS"
          :key="t.key"
          type="button"
          class="-mb-px border-b-2 px-4 py-2 text-sm transition"
          :class="tab === t.key
            ? 'border-primary-500 font-semibold text-primary-600'
            : 'border-transparent text-gray-500 hover:text-gray-800 dark:hover:text-white'"
          @click="tab = t.key"
        >{{ t.label }}</button>
      </nav>

      <div v-if="!settings.loaded" class="card p-10 text-center text-gray-400">{{ loadError || '加载中…' }}</div>
      <template v-else>
        <BasicTab v-show="tab === 'basic'" />
        <LayoutTab v-show="tab === 'layout'" />
        <RulesTab v-show="tab === 'rules'" />
        <ProbeTab v-show="tab === 'probe'" />
      </template>
    </div>

    <footer
      v-if="settings.loaded"
      class="fixed inset-x-0 bottom-0 z-30 border-t border-gray-200 bg-white/90 backdrop-blur dark:border-dark-700 dark:bg-dark-900/90"
    >
      <div class="mx-auto flex max-w-6xl items-center gap-3 px-6 py-3">
        <span class="mr-auto text-xs text-gray-400">
          配置版本 v{{ settings.version }}
          <span v-if="dirty" class="ml-2 text-amber-600">· 有未保存的修改</span>
        </span>
        <button type="button" class="btn btn-secondary" :disabled="!dirty || settings.saving" @click="resetSettings">重置</button>
        <button type="button" class="btn btn-primary" :disabled="!dirty || settings.saving" @click="save">
          {{ settings.saving ? '保存中…' : '保存设置' }}
        </button>
      </div>
    </footer>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { CHANNEL_STATUS_PATH } from '../routes'
import { dirty, loadSettings, resetSettings, saveSettings, settings, validateDraft } from './state'
import BasicTab from './BasicTab.vue'
import LayoutTab from './LayoutTab.vue'
import RulesTab from './RulesTab.vue'
import ProbeTab from './ProbeTab.vue'

const TABS = [
  { key: 'basic', label: '基础设置' },
  { key: 'layout', label: '展示编排' },
  { key: 'rules', label: '健康规则' },
  { key: 'probe', label: '兜底探测' }
] as const
type TabKey = (typeof TABS)[number]['key']

const appStore = useAppStore()
const tab = ref<TabKey>('basic')
const loadError = ref('')

onMounted(async () => {
  try {
    await loadSettings()
  } catch (e) {
    loadError.value = extractApiErrorMessage(e, '加载失败')
  }
})

async function save() {
  if (!settings.draft) return
  const [where, problem] = validateDraft(settings.draft)
  if (problem) {
    tab.value = where as TabKey
    appStore.showError(problem)
    return
  }
  try {
    await saveSettings()
    appStore.showSuccess('已保存')
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '保存失败'))
  }
}
</script>
