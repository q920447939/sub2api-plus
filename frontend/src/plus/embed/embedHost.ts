/**
 * [plus] Keeps embedded plus apps alive across route changes.
 *
 * Every view renders its own AppLayout, so an iframe placed inside LaunchView is destroyed the
 * moment the user opens another menu. <KeepAlive> does not help: it detaches the DOM, and a
 * detached iframe reloads when re-attached. For an image generator that means the running
 * request is lost.
 *
 * Instead the iframes live in PlusEmbedHost (mounted once in App.vue). LaunchView only renders an
 * empty placeholder and reports its on-screen rectangle; the host lays the active iframe over it
 * and hides the others with display:none, which keeps their scripts and requests running.
 *
 * The frames array is never reordered: moving an iframe node in the DOM also reloads it. Recency
 * is tracked with a counter instead.
 */
import { reactive, readonly } from 'vue'

export interface EmbedRect {
  top: number
  left: number
  width: number
  height: number
}

export interface EmbedFrame {
  menuId: string
  url: string
  /** user the frame was resolved for; frames never survive a user switch */
  ownerId: number | null
  hideOpenButton: boolean
  loaded: boolean
  usedAt: number
}

/** Upper bound on live iframes; the least recently used one is dropped beyond this. */
export const MAX_LIVE_FRAMES = 3

interface EmbedHostState {
  frames: EmbedFrame[]
  activeId: string | null
  rect: EmbedRect | null
}

const state = reactive<EmbedHostState>({
  frames: [],
  activeId: null,
  rect: null,
})

let clock = 0

export const embedHostState = readonly(state)

export function getFrame(menuId: string, ownerId: number | null): EmbedFrame | undefined {
  return state.frames.find((f) => f.menuId === menuId && f.ownerId === ownerId)
}

/** Adds the frame, or replaces it in place when the target changed, and marks it used. */
export function openFrame(frame: Omit<EmbedFrame, 'loaded' | 'usedAt'>): void {
  const usedAt = ++clock
  const existing = state.frames.find((f) => f.menuId === frame.menuId)
  if (existing) {
    if (existing.url !== frame.url || existing.ownerId !== frame.ownerId) existing.loaded = false
    Object.assign(existing, frame, { usedAt })
    return
  }
  state.frames.push({ ...frame, loaded: false, usedAt })
  while (state.frames.length > MAX_LIVE_FRAMES) {
    let oldest = 0
    state.frames.forEach((f, i) => {
      if (f.usedAt < state.frames[oldest].usedAt) oldest = i
    })
    closeFrame(state.frames[oldest].menuId)
  }
}

export function touchFrame(menuId: string): void {
  const frame = state.frames.find((f) => f.menuId === menuId)
  if (frame) frame.usedAt = ++clock
}

export function markLoaded(menuId: string): void {
  const frame = state.frames.find((f) => f.menuId === menuId)
  if (frame) frame.loaded = true
}

export function activate(menuId: string, rect: EmbedRect): void {
  state.activeId = menuId
  const r = state.rect
  if (!r || r.top !== rect.top || r.left !== rect.left || r.width !== rect.width || r.height !== rect.height) {
    state.rect = { ...rect }
  }
}

/** Hides the frame without destroying it. Only the view that showed it can hide it. */
export function deactivate(menuId: string): void {
  if (state.activeId === menuId) state.activeId = null
}

export function closeFrame(menuId: string): void {
  const idx = state.frames.findIndex((f) => f.menuId === menuId)
  if (idx >= 0) state.frames.splice(idx, 1)
  if (state.activeId === menuId) state.activeId = null
}

export function clearFrames(): void {
  state.frames.splice(0)
  state.activeId = null
  state.rect = null
}
