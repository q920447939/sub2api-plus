// [plus] 渠道状态数据：按粒度拉取快照，按后端给的周期轮询；页面不可见时暂停
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { getSnapshot, type Granularity, type Snapshot } from './api'

export function useChannelStatus() {
  const snapshot = ref<Snapshot | null>(null)
  const granularity = ref<Granularity | null>(null) // null = 用后端默认
  const loading = ref(false)
  const disabled = ref(false)
  const error = ref('')
  let timer: ReturnType<typeof setTimeout> | null = null
  let seq = 0

  async function load() {
    const my = ++seq
    loading.value = true
    try {
      const snap = await getSnapshot(granularity.value ?? undefined)
      if (my !== seq) return
      snapshot.value = snap
      if (!granularity.value) granularity.value = snap.granularity
      disabled.value = false
      error.value = ''
    } catch (e) {
      if (my !== seq) return
      const status = (e as { status?: number })?.status
      if (status === 404) disabled.value = true
      else error.value = '加载失败，稍后自动重试'
    } finally {
      if (my === seq) loading.value = false
      schedule()
    }
  }

  function schedule() {
    if (timer) clearTimeout(timer)
    const secs = snapshot.value?.refresh_seconds ?? 60
    timer = setTimeout(() => {
      if (document.visibilityState === 'visible') void load()
      else schedule()
    }, secs * 1000)
  }

  function onVisible() {
    if (document.visibilityState === 'visible') void load()
  }

  watch(granularity, (g, old) => {
    if (old && g !== old) void load()
  })

  onMounted(() => {
    void load()
    document.addEventListener('visibilitychange', onVisible)
  })
  onBeforeUnmount(() => {
    if (timer) clearTimeout(timer)
    document.removeEventListener('visibilitychange', onVisible)
  })

  return { snapshot, granularity, loading, disabled, error, reload: load }
}
