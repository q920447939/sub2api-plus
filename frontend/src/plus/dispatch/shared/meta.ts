// [plus] 调度台注册表（检测方式、指标、维度、检测项、计划、平台和模型），各页面共享
import { ref } from 'vue'
import { dispatchApi, type Meta } from '../api'

export const meta = ref<Meta | null>(null)
let inflight: Promise<Meta> | null = null

export async function loadMeta(force = false): Promise<Meta> {
  if (meta.value && !force) return meta.value
  if (!inflight || force) {
    inflight = dispatchApi.meta().finally(() => {
      inflight = null
    })
  }
  meta.value = await inflight
  return meta.value
}

export const specOf = (type: string) => meta.value?.types.find((t) => t.key === type)
export const metricOf = (key: string) => meta.value?.metrics.find((m) => m.key === key)
export const itemOf = (id: number) => meta.value?.items.find((i) => i.id === id)
export const dimOf = (id: number) => meta.value?.dimensions.find((d) => d.id === id)
