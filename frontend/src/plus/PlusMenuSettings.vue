<!--
  [plus] Admin editor for a custom menu item's fork-specific options.

  Rendered inside the menu item card in SettingsView. It owns three things:
    - open_mode:      embed in the iframe page, or open in a new tab
    - pass_params:    whether the launcher appends user_id/token/theme/lang
    - plus_config:    module configuration, shown only for `plus:key_launch` menus

  The config is saved together with the menu item by the page's normal save button, so there
  is no second save path to reason about.
-->
<template>
  <div class="sm:col-span-2 space-y-3 rounded-lg border border-dashed border-gray-300 p-3 dark:border-dark-600">
    <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.settings.plusMenu.sectionTitle') }}</p>

    <!-- Open mode -->
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
      <div>
        <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
          {{ t('admin.settings.plusMenu.openMode') }}
        </label>
        <select :value="openModeValue" class="input" :data-testid="`plus-open-mode-${item.id}`" @change="onOpenModeChange">
          <option value="embed">{{ t('admin.settings.plusMenu.openModeEmbed') }}</option>
          <option value="new_tab">{{ t('admin.settings.plusMenu.openModeNewTab') }}</option>
        </select>
      </div>

      <label class="flex items-end gap-2 pb-2">
        <input
          :checked="passParamsValue"
          type="checkbox"
          :data-testid="`plus-pass-params-${item.id}`"
          @change="onPassParamsChange"
        />
        <span class="text-sm text-gray-700 dark:text-gray-300">
          {{ t('admin.settings.plusMenu.passParams') }}
        </span>
      </label>
    </div>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.plusMenu.passParamsHint') }}</p>

    <!-- Module configuration -->
    <template v-if="isKeyLaunch">
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
            {{ t('admin.settings.plusMenu.group') }}
          </label>
          <select
            :value="plusConfig.group_id || ''"
            class="input"
            :data-testid="`plus-group-${item.id}`"
            @change="onGroupChange"
          >
            <option value="">{{ t('admin.settings.plusMenu.groupPlaceholder') }}</option>
            <option v-for="g in groups" :key="g.id" :value="g.id">
              {{ g.name }} (#{{ g.id }})
            </option>
          </select>
        </div>

        <div>
          <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
            {{ t('admin.settings.plusMenu.rechargeMenu') }}
          </label>
          <select
            :value="plusConfig.recharge_menu_id || ''"
            class="input"
            :data-testid="`plus-recharge-${item.id}`"
            @change="onRechargeChange"
          >
            <option value="">{{ t('admin.settings.plusMenu.rechargeDefault') }}</option>
            <option v-for="m in otherMenuItems" :key="m.id" :value="m.id">
              {{ m.label }}
            </option>
          </select>
        </div>
      </div>

      <div>
        <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
          {{ t('admin.settings.plusMenu.urlTemplate') }}
        </label>
        <input
          :value="plusConfig.url_template"
          type="text"
          class="input font-mono text-sm"
          :placeholder="t('admin.settings.plusMenu.urlTemplatePlaceholder')"
          :data-testid="`plus-url-template-${item.id}`"
          @input="onTemplateInput"
        />
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.plusMenu.urlTemplateHint') }}</p>
      </div>

      <p
        v-if="hint"
        class="text-xs text-red-500"
      >
        {{ hint }}
      </p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getAll } from '@/api/admin/groups'
import type { AdminGroup } from '@/types'

interface PlusConfig {
  group_id: number
  url_template: string
  recharge_menu_id?: string
}

interface MenuItemLike {
  id: string
  label: string
  url: string
  open_mode?: 'embed' | 'new_tab'
  pass_params?: boolean
  plus_config?: PlusConfig
}

const props = defineProps<{
  item: MenuItemLike
  menuItems: MenuItemLike[]
}>()

// The parent owns the menu item (it is part of the settings form and is saved with the normal
// save button), so this component only emits the new values.
const emit = defineEmits<{
  change: [patch: Partial<MenuItemLike>]
}>()

const { t } = useI18n()

const groups = ref<AdminGroup[]>([])

const isKeyLaunch = computed(() => props.item.url?.trim().startsWith('plus:key_launch'))

const otherMenuItems = computed(() => props.menuItems.filter((m) => m.id !== props.item.id))

// No side effects: a missing config is simply treated as empty until the user edits it.
const plusConfig = computed<PlusConfig>(
  () => props.item.plus_config ?? { group_id: 0, url_template: '' },
)

const templateValid = computed(() => {
  const tpl = plusConfig.value.url_template ?? ''
  if (!tpl.includes('{key}')) return false
  try {
    new URL(tpl.replace('{key}', 'probe'))
    return true
  } catch {
    return false
  }
})

const openModeValue = computed(() => props.item.open_mode ?? 'embed')
const passParamsValue = computed(() => props.item.pass_params !== false)

// Shown while a key-launch menu is still incomplete, so the admin sees what is missing before
// the backend rejects the save.
const hint = computed(() => {
  if (!isKeyLaunch.value) return ''
  const missing: string[] = []
  if (!plusConfig.value.group_id) missing.push(t('admin.settings.plusMenu.hintGroup'))
  if (!templateValid.value) missing.push(t('admin.settings.plusMenu.hintTemplate'))
  return missing.length
    ? t('admin.settings.plusMenu.hintMissing', { fields: missing.join('、') })
    : ''
})

function onOpenModeChange(e: Event) {
  emit('change', { open_mode: (e.target as HTMLSelectElement).value as 'embed' | 'new_tab' })
}

function onPassParamsChange(e: Event) {
  emit('change', { pass_params: (e.target as HTMLInputElement).checked })
}

function onGroupChange(e: Event) {
  emit('change', {
    plus_config: {
      ...plusConfig.value,
      group_id: Number((e.target as HTMLSelectElement).value) || 0,
    },
  })
}

function onRechargeChange(e: Event) {
  emit('change', {
    plus_config: {
      ...plusConfig.value,
      recharge_menu_id: (e.target as HTMLSelectElement).value || undefined,
    },
  })
}

function onTemplateInput(e: Event) {
  emit('change', {
    plus_config: {
      ...plusConfig.value,
      url_template: (e.target as HTMLInputElement).value,
    },
  })
}

async function loadGroups() {
  if (groups.value.length > 0) return // already loaded
  try {
    groups.value = await getAll()
  } catch {
    groups.value = []
  }
}

onMounted(() => {
  if (isKeyLaunch.value) void loadGroups()
})

// Load groups when the admin switches the URL to plus:key_launch
watch(isKeyLaunch, (v) => {
  if (v) void loadGroups()
})
</script>
