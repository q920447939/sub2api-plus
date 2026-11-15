<!--
  [plus] LaunchView resolves a plus menu item and then opens the target.

  Both open modes route here:
  - embed:      the target runs in a persistent iframe owned by PlusEmbedHost (see plus/embed).
                This view only renders a placeholder and reports where the iframe should sit, so
                switching to another menu and back does not reload the app or drop its work.
  - new_tab:    the browser opens this page in a new tab and we redirect it to the target

  Resolution happens in the backend (see backend/internal/plus), so credentials never travel
  through the frontend. A refused launch renders the reason plus an action button.
-->
<template>
  <AppLayout>
    <div class="launch-layout">
      <!-- Resolving -->
      <div v-if="state === 'loading'" class="launch-center">
        <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
        <p class="launch-hint">{{ t('plus.resolving') }}</p>
      </div>

      <!-- Blocked: explain why and offer the next step -->
      <div v-else-if="state === 'blocked'" class="launch-center">
        <div class="launch-card">
          <div class="launch-card-icon">
            <Icon name="exclamationCircle" size="lg" class="text-amber-500" />
          </div>
          <h3 class="launch-card-title">{{ blockedTitle }}</h3>
          <p class="launch-card-message">{{ blockedMessage }}</p>
          <div class="launch-card-actions">
            <button v-if="blockedAction" type="button" class="btn btn-primary" @click="goToAction">
              {{ blockedAction.label }}
            </button>
            <button type="button" class="btn btn-secondary" @click="retry">{{ t('plus.retry') }}</button>
          </div>
        </div>
      </div>

      <!-- Embed mode: placeholder the persistent iframe is laid over -->
      <div
        v-else-if="state === 'ready' && openMode !== 'new_tab'"
        ref="embedShell"
        class="launch-embed-shell"
        data-testid="launch-embed-shell"
      ></div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { launchMenu, type LaunchBlockAction } from '@/plus/api'
import { activate, deactivate, getFrame, openFrame, touchFrame } from '@/plus/embed/embedHost'
import { useAdminSettingsStore, useAppStore, useAuthStore } from '@/stores'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()
const adminSettingsStore = useAdminSettingsStore()

type LaunchState = 'loading' | 'blocked' | 'ready'

const state = ref<LaunchState>('loading')
const blockedMessage = ref('')
const blockedTitle = ref('')
const blockedAction = ref<LaunchBlockAction | undefined>(undefined)
const embedShell = ref<HTMLElement | null>(null)

const menuId = computed(() => String(route.params.id ?? ''))
const ownerId = computed(() => authStore.user?.id ?? null)

// [plus] public settings strip plus_config but keep open_mode, so this works for regular users.
// Admin-only menus are not in public settings; admins read them from the admin store.
const menuItem = computed(() => {
  const id = menuId.value
  const found = appStore.cachedPublicSettings?.custom_menu_items?.find((m) => m.id === id)
  if (found) return found
  if (authStore.isAdmin) return adminSettingsStore.customMenuItems.find((m) => m.id === id)
  return undefined
})

const openMode = computed<'embed' | 'new_tab'>(() => menuItem.value?.open_mode ?? 'embed')

let resolveVersion = 0

async function resolve(force = false) {
  const id = menuId.value
  const version = ++resolveVersion

  // Already running in this session: show it again instead of reloading it.
  if (!force && openMode.value !== 'new_tab' && getFrame(id, ownerId.value)) {
    touchFrame(id)
    state.value = 'ready'
    return
  }

  state.value = 'loading'
  const outcome = await launchMenu(id)
  if (version !== resolveVersion) return // the user moved to another menu meanwhile

  if (outcome.kind === 'ok') {
    if (openMode.value === 'new_tab') {
      state.value = 'ready'
      // This page was opened in a new tab; hand it over to the real target.
      window.location.replace(outcome.url)
      return
    }
    openFrame({
      menuId: id,
      url: outcome.url,
      ownerId: ownerId.value,
      hideOpenButton: menuItem.value?.hide_open_button === true,
    })
    state.value = 'ready'
    return
  }
  blockedTitle.value = t('plus.blockedTitle')
  blockedMessage.value = outcome.reason.message
  blockedAction.value = outcome.reason.action
  state.value = 'blocked'
}

function goToAction() {
  if (blockedAction.value) {
    window.location.href = blockedAction.value.path
  }
}

function retry() {
  void resolve(true)
}

// Follow the placeholder every frame: sidebar collapse, window resize and layout shifts all
// move it, and a stale rectangle would leave the iframe floating in the wrong place.
let raf = 0
let shownId: string | null = null

function track() {
  const el = embedShell.value
  const id = menuId.value
  if (el && state.value === 'ready' && getFrame(id, ownerId.value)) {
    const r = el.getBoundingClientRect()
    activate(id, { top: r.top, left: r.left, width: r.width, height: r.height })
    shownId = id
  } else if (shownId) {
    deactivate(shownId)
    shownId = null
  }
  raf = requestAnimationFrame(track)
}

// Same component instance is reused for /launch/a -> /launch/b.
watch(menuId, (_next, prev) => {
  if (prev) deactivate(prev)
  shownId = null
  void resolve()
})

onMounted(() => {
  void resolve()
  raf = requestAnimationFrame(track)
})

onBeforeUnmount(() => {
  resolveVersion++
  cancelAnimationFrame(raf)
  if (shownId) deactivate(shownId)
})
</script>

<style scoped>
.launch-layout {
  @apply flex flex-col items-center justify-center;
  height: calc(100vh - 64px - 4rem);
}

.launch-center {
  @apply flex flex-col items-center justify-center gap-4 text-center;
}

.launch-hint {
  @apply text-sm text-gray-500 dark:text-dark-400;
}

.launch-card {
  @apply max-w-md rounded-2xl border border-gray-200 bg-white p-8 shadow-sm dark:border-dark-600 dark:bg-dark-800;
}

.launch-card-icon {
  @apply mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-amber-50 dark:bg-amber-900/20;
}

.launch-card-title {
  @apply text-lg font-semibold text-gray-900 dark:text-white;
}

.launch-card-message {
  @apply mt-2 text-sm text-gray-500 dark:text-dark-400;
}

.launch-card-actions {
  @apply mt-6 flex items-center justify-center gap-3;
}

.launch-embed-shell {
  @apply relative h-full w-full overflow-hidden rounded-2xl;
}
</style>
