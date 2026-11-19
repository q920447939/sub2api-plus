// [plus] 编辑器实时预览：防抖调用后端渲染（与实际发信同一条管线），只保留最后一次结果
import { onBeforeUnmount, reactive, watch, type WatchSource } from 'vue'
import { adminApi, type RenderRequest } from './api'

export function useMailRender(source: WatchSource<RenderRequest | null>) {
  const rendered = reactive({ subject: '', html: '', error: '' })
  let timer: ReturnType<typeof setTimeout> | null = null
  let seq = 0

  function run(req: RenderRequest | null) {
    if (timer) clearTimeout(timer)
    if (!req || !req.purpose) return
    timer = setTimeout(async () => {
      const mine = ++seq
      try {
        const r = await adminApi.render(JSON.parse(JSON.stringify(req)) as RenderRequest)
        if (mine === seq) Object.assign(rendered, r, { error: '' })
      } catch {
        if (mine === seq) rendered.error = '预览渲染失败'
      }
    }, 400)
  }

  watch(source, run, { deep: true, immediate: true })
  onBeforeUnmount(() => timer && clearTimeout(timer))
  return rendered
}
