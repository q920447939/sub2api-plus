<!--
  [plus] Persistent layer for embedded plus apps. Mounted once in App.vue; see embedHost.ts.

  The active frame is laid over LaunchView's placeholder with position:fixed. Inactive frames
  stay in the DOM with display:none so in-flight work (e.g. an image generation) keeps running.
-->
<template>
  <div v-if="state.frames.length" class="plus-embed-host" aria-hidden="false">
    <div
      v-for="frame in state.frames"
      v-show="frame.menuId === state.activeId && state.rect"
      :key="frame.menuId + '|' + frame.url"
      class="plus-embed-slot"
      :style="slotStyle"
      :data-menu-id="frame.menuId"
    >
      <div v-if="!frame.loaded" class="plus-embed-loading">
        <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
      </div>
      <a
        v-if="!frame.hideOpenButton"
        :href="frame.url"
        target="_blank"
        rel="noopener noreferrer"
        class="btn btn-secondary btn-sm plus-embed-open"
        :style="buttonStyle(frame.menuId)"
        @pointerdown="startDrag($event, frame.menuId)"
        @pointermove="moveDrag"
        @pointerup="endDrag"
        @pointercancel="endDrag"
        @lostpointercapture="endDrag"
        @dragstart.prevent
        @click="onOpenClick"
      >
        <Icon name="externalLink" size="sm" class="mr-1.5" :stroke-width="2" />
        {{ t('customPage.openInNewTab') }}
      </a>
      <!--
        sandbox: everything an app normally needs, but no allow-top-navigation, so the embedded
        page cannot navigate the whole platform away. allow-same-origin keeps the app's own
        storage (IndexedDB); it is safe because the app is on a different origin.
      -->
      <iframe
        :src="frame.url"
        class="plus-embed-frame"
        allow="clipboard-read; clipboard-write; fullscreen"
        allowfullscreen
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-popups-to-escape-sandbox allow-downloads allow-modals"
        referrerpolicy="no-referrer"
        @load="markLoaded(frame.menuId)"
      ></iframe>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useAuthStore } from '@/stores/auth'
import { clearFrames, embedHostState as state, markLoaded } from './embedHost'

const { t } = useI18n()
const authStore = useAuthStore()

// The URL carries the user's key; never let a frame outlive the session that created it.
watch(() => authStore.token, (token) => {
  if (!token) clearFrames()
})
watch(() => authStore.user?.id ?? null, (id, prev) => {
  if (prev != null && id !== prev) clearFrames()
})

const slotStyle = computed(() => {
  const r = state.rect
  if (!r) return {}
  return { top: `${r.top}px`, left: `${r.left}px`, width: `${r.width}px`, height: `${r.height}px` }
})

// Draggable "open in new tab" button, so it never permanently covers the app's own controls.
const buttonPos = reactive<Record<string, { x: number; y: number }>>({})
let drag: { id: string; pointerId: number; sx: number; sy: number; x: number; y: number; el: HTMLElement } | null = null
let suppressClick = false

function buttonStyle(menuId: string) {
  const p = buttonPos[menuId]
  return p ? { left: `${p.x}px`, top: `${p.y}px`, right: 'auto' } : undefined
}

function startDrag(e: PointerEvent, menuId: string) {
  if (e.button !== 0 || !e.isPrimary) return
  const el = e.currentTarget as HTMLElement
  suppressClick = false
  drag = { id: menuId, pointerId: e.pointerId, sx: e.clientX, sy: e.clientY, x: el.offsetLeft, y: el.offsetTop, el }
  // capture keeps receiving moves while the pointer crosses the iframe
  el.setPointerCapture(e.pointerId)
}

function moveDrag(e: PointerEvent) {
  if (!drag || e.pointerId !== drag.pointerId) return
  const dx = e.clientX - drag.sx
  const dy = e.clientY - drag.sy
  if (!suppressClick && Math.hypot(dx, dy) < 4) return
  suppressClick = true
  const parent = drag.el.parentElement
  const maxX = (parent?.clientWidth ?? 0) - drag.el.offsetWidth
  const maxY = (parent?.clientHeight ?? 0) - drag.el.offsetHeight
  buttonPos[drag.id] = {
    x: Math.max(0, Math.min(drag.x + dx, maxX)),
    y: Math.max(0, Math.min(drag.y + dy, maxY)),
  }
  e.preventDefault()
}

function endDrag(e: PointerEvent) {
  if (!drag || e.pointerId !== drag.pointerId) return
  if (drag.el.hasPointerCapture(e.pointerId)) drag.el.releasePointerCapture(e.pointerId)
  drag = null
}

function onOpenClick(e: MouseEvent) {
  if (suppressClick && e.detail !== 0) e.preventDefault()
  suppressClick = false
}
</script>

<style scoped>
/* below the header (z-30) and mobile sidebar, so their dropdowns and drawers stay on top */
.plus-embed-host {
  position: relative;
  z-index: 20;
}

.plus-embed-slot {
  position: fixed;
  z-index: 20;
  overflow: hidden;
  border-radius: 1rem;
}

.plus-embed-frame {
  display: block;
  width: 100%;
  height: 100%;
  border: 0;
  background: transparent;
}

.plus-embed-loading {
  @apply absolute inset-0 flex items-center justify-center;
  pointer-events: none;
}

.plus-embed-open {
  position: absolute;
  top: 0.75rem;
  right: 0.75rem;
  z-index: 1;
  cursor: grab;
  touch-action: none;
  user-select: none;
  @apply shadow-md;
}
</style>
