<!--
  [plus] LaunchView resolves a plus menu item and then opens the target.

  Both open modes route here:
  - embed:      the resolved URL is rendered in an iframe inside the app layout
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

      <!-- Embed mode: render the resolved target inline -->
      <div v-else-if="state === 'ready' && openMode !== 'new_tab'" ref="embedShell" class="launch-embed-shell">
        <iframe :src="targetUrl" class="launch-embed-frame" allowfullscreen></iframe>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { launchMenu, type LaunchBlockAction } from '@/plus/api'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()

type LaunchState = 'loading' | 'blocked' | 'ready'

const state = ref<LaunchState>('loading')
const targetUrl = ref('')
const blockedMessage = ref('')
const blockedTitle = ref('')
const blockedAction = ref<LaunchBlockAction | undefined>(undefined)

const menuId = computed(() => String(route.params.id ?? ''))

// [plus] Read open_mode from public settings so this page knows whether to embed or redirect.
// public settings strip plus_config but keep open_mode, so this works for regular users.
const openMode = computed<'embed' | 'new_tab'>(() => {
  const item = appStore.cachedPublicSettings?.custom_menu_items?.find(
    (m) => m.id === menuId.value,
  )
  return item?.open_mode ?? 'embed'
})

async function resolve() {
  state.value = 'loading'
  const outcome = await launchMenu(menuId.value)
  if (outcome.kind === 'ok') {
    targetUrl.value = outcome.url
    state.value = 'ready'
    if (openMode.value === 'new_tab') {
      // This page was opened in a new tab; hand it over to the real target.
      window.location.replace(outcome.url)
    }
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
  void resolve()
}

onMounted(() => {
  void resolve()
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

.launch-embed-frame {
  display: block;
  width: 100%;
  height: 100%;
  border: 0;
}
</style>
