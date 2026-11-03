<template>
  <AppLayout>
    <div class="space-y-6 pb-20">
      <div>
        <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ title }}</h1>
        <p v-if="subtitle" class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ subtitle }}</p>
      </div>

      <div v-if="tabs.length > 1" class="border-b border-gray-200 dark:border-dark-700" role="tablist">
        <nav class="-mb-px flex flex-wrap gap-6">
          <button
            v-for="t in tabs"
            :key="t.key"
            type="button"
            role="tab"
            :aria-selected="tab === t.key"
            class="border-b-2 px-1 pb-3 text-sm font-medium transition-colors"
            :class="
              tab === t.key
                ? 'border-primary-500 text-primary-600 dark:text-primary-400'
                : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'
            "
            :data-testid="`park-admin-tab-${t.key}`"
            @click="setTab(t.key)"
          >{{ t.label }}</button>
        </nav>
      </div>

      <!-- 配置类标签页依赖三份配置（互相引用），统一加载 -->
      <template v-if="isConfigTab">
        <div v-if="loadError" class="card p-6 text-center">
          <p class="text-rose-500">{{ loadError }}</p>
          <button type="button" class="btn btn-secondary mt-3" @click="load">重试</button>
        </div>
        <div v-else-if="!park.loaded" class="card p-6 text-center text-gray-400">加载中…</div>
        <div v-else :key="tab"><slot :name="tab" /></div>
      </template>
      <div v-else :key="tab"><slot :name="tab" /></div>

      <div
        v-if="isConfigTab && park.loaded && section"
        class="sticky bottom-0 z-10 flex flex-wrap items-center justify-end gap-3 border-t border-gray-200 bg-white/90 py-3 backdrop-blur dark:border-dark-700 dark:bg-dark-900/90"
      >
        <span v-if="problems.length" class="mr-auto text-sm text-rose-500">{{ problems[0] }}</span>
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="load">放弃修改</button>
        <button type="button" class="btn btn-primary" :disabled="saving || problems.length > 0" data-testid="park-admin-save" @click="save">
          {{ saving ? '保存中…' : saveLabel }}
        </button>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { loadParkAdmin, park, saveParkSection, sectionProblems, type ParkSection } from './adminState'

const props = withDefaults(
  defineProps<{
    title: string
    subtitle?: string
    tabs: { key: string; label: string }[]
    /** 哪些标签页是配置页（显示保存栏、需要加载配置） */
    configTabs?: string[]
    section?: ParkSection
    saveLabel?: string
  }>(),
  { subtitle: '', configTabs: () => [], section: undefined, saveLabel: '保存配置' }
)

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()

// 当前标签记在 URL 上，刷新后保持
const tab = computed(() => {
  const q = route.query.tab
  return typeof q === 'string' && props.tabs.some((t) => t.key === q) ? q : props.tabs[0]?.key ?? ''
})
function setTab(key: string) {
  if (key !== tab.value) void router.replace({ query: { ...route.query, tab: key } })
}

const isConfigTab = computed(() => props.configTabs.includes(tab.value))
const problems = computed(() => (props.section ? sectionProblems(props.section) : []))

const loadError = ref('')
const saving = ref(false)

async function load() {
  loadError.value = ''
  try {
    await loadParkAdmin()
  } catch (e) {
    loadError.value = extractApiErrorMessage(e, '加载失败')
  }
}

async function save() {
  if (!props.section) return
  saving.value = true
  try {
    await saveParkSection(props.section)
    appStore.showSuccess('已保存')
  } catch (e) {
    appStore.showError(extractApiErrorMessage(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

// 进入配置页时总是拉最新配置，避免沿用其他页面未保存的编辑
watch(
  isConfigTab,
  (v) => {
    if (v) void load()
  }
)
onMounted(() => {
  if (isConfigTab.value) void load()
})
</script>
