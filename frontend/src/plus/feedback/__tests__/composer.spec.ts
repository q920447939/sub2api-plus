import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

const post = vi.fn()
vi.mock('@/api/client', () => ({ apiClient: { get: vi.fn(() => Promise.reject(new Error('offline'))), post: (...a: unknown[]) => post(...a), put: vi.fn(), patch: vi.fn() } }))
vi.mock('vue-router', () => ({ useRouter: () => ({ currentRoute: { value: { path: '/keys' } }, push: vi.fn() }) }))
vi.mock('../api', async (importOriginal) => ({ ...(await importOriginal<typeof import('../api')>()), imageURL: () => Promise.resolve('blob:test') }))

import Composer from '../components/Composer.vue'
import Sheet from '../components/Sheet.vue'
import AuthImage from '../components/AuthImage.vue'
import { composer, draft, meta, resetDraft } from '../state'
import type { Meta } from '../api'

// Sheet 用 Teleport 渲染到 body，测试里直接渲染插槽，并留一个关闭按钮模拟 X / Esc / 点遮罩
const stubs = {
  Sheet: {
    props: ['open'],
    emits: ['close'],
    template: '<div v-if="open"><button data-testid="sheet-close" @click="$emit(\'close\')" /><slot /><slot name="footer" /></div>'
  },
  ImagePicker: {
    name: 'ImagePicker',
    props: ['modelValue', 'lateAdd'],
    emits: ['update:modelValue', 'update:uploading'],
    template: '<div />'
  },
  NoticeCard: true
}

function setup(patch: Partial<Meta> = {}) {
  meta.value = {
    enabled: true,
    float_button: true,
    notice: '',
    max_images: 3,
    max_image_mb: 5,
    content_min: 10,
    content_max: 2000,
    scenarios: [
      { key: 'call_error', kind: 'issue', label: '调用报错', icon: '⚠️', hint: '', fields: [] },
      { key: 'ux', kind: 'suggestion', label: '体验可以更好', icon: '🎯', hint: '', fields: [] }
    ],
    ask_timeout_days: 7,
    ...patch
  }
  const w = mount(Composer, { global: { stubs } })
  composer.open = true
  return w
}

describe('提交面板校验', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    post.mockReset()
    composer.open = false
    resetDraft()
  })

  it('打开时不显示错误；点提交后标出缺少场景和描述，且不发请求', async () => {
    const w = setup()
    await flushPromises()
    expect(w.findAll('[role="alert"]')).toHaveLength(0)

    await w.get('[data-testid="composer-submit"]').trigger('click')
    expect(w.text()).toContain('请选择问题的场景')
    expect(post).not.toHaveBeenCalled()

    await w.findAll('button').find((b) => b.text().includes('调用报错'))!.trigger('click')
    expect(w.text()).not.toContain('请选择问题的场景')
    await w.get('[data-testid="composer-submit"]').trigger('click')
    expect(w.text()).toContain('请描述一下遇到的情况')
    expect(post).not.toHaveBeenCalled()
  })

  it('字数不够时提示还差几个字，输够后提示消失并提交', async () => {
    const w = setup()
    await flushPromises()
    await w.findAll('button').find((b) => b.text().includes('调用报错'))!.trigger('click')
    const ta = w.get('[data-testid="composer-content"]')
    await ta.setValue('报错了')
    expect(w.get('[data-testid="composer-counter"]').text()).toContain('还差 7 字')

    await w.get('[data-testid="composer-submit"]').trigger('click')
    expect(w.text()).toContain('至少 10 个字，还差 7 个')

    post.mockResolvedValue({ data: { id: 42 } })
    await ta.setValue('调用 claude 一直返回 502 错误')
    expect(w.findAll('[role="alert"]')).toHaveLength(0)
    await w.get('[data-testid="composer-submit"]').trigger('click')
    await flushPromises()
    expect(post).toHaveBeenCalledOnce()
  })

  it('服务端拒绝时，错误显示在提交按钮上方，草稿保留', async () => {
    const w = setup()
    await flushPromises()
    await w.findAll('button').find((b) => b.text().includes('调用报错'))!.trigger('click')
    await w.get('[data-testid="composer-content"]').setValue('调用 claude 一直返回 502 错误')
    post.mockRejectedValue({ message: '提交太快了，30 秒后再试' })
    await w.get('[data-testid="composer-submit"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-testid="composer-error"]').text()).toContain('提交太快了')
    expect((w.get('[data-testid="composer-content"]').element as HTMLTextAreaElement).value).toContain('502')
  })

  it('图片还在上传时不能提交（按钮和 Ctrl+Enter 都不行），传完后带上图片', async () => {
    const w = setup()
    await flushPromises()
    await pick(w, '调用报错')
    const ta = w.get('[data-testid="composer-content"]')
    await ta.setValue('调用 claude 一直返回 502 错误')

    const picker = w.findComponent({ name: 'ImagePicker' })
    picker.vm.$emit('update:uploading', 1)
    await flushPromises()
    const btn = w.get('[data-testid="composer-submit"]')
    expect(btn.attributes('disabled')).toBeDefined()
    expect(btn.text()).toBe('图片上传中…')
    await ta.trigger('keydown', { key: 'Enter', ctrlKey: true })
    await flushPromises()
    expect(post).not.toHaveBeenCalled()

    picker.vm.$emit('update:modelValue', [7])
    picker.vm.$emit('update:uploading', 0)
    await flushPromises()
    expect(btn.attributes('disabled')).toBeUndefined()
    post.mockResolvedValue({ data: { id: 1 } })
    await btn.trigger('click')
    await flushPromises()
    expect(post.mock.calls[0][1]).toMatchObject({ attachments: [7] })
  })

  it('面板关掉后才传完的图片仍写回草稿', async () => {
    const w = setup()
    await flushPromises()
    draft.images = [3]
    const lateAdd = w.findComponent({ name: 'ImagePicker' }).props('lateAdd') as (id: number) => void
    composer.open = false
    await flushPromises()
    lateAdd(5)
    expect(draft.images).toEqual([3, 5])
  })

  it('关掉面板、换页重建后草稿还在；提交成功后才清空', async () => {
    let w = setup()
    await flushPromises()
    await pick(w, '调用报错')
    await w.get('[data-testid="composer-content"]').setValue('调用 claude 一直返回 502 错误')
    composer.open = false
    w.unmount()

    w = setup()
    await flushPromises()
    expect((w.get('[data-testid="composer-content"]').element as HTMLTextAreaElement).value).toContain('502')
    expect(w.findAll('button').find((b) => b.text().includes('调用报错'))!.attributes('aria-pressed')).toBe('true')

    post.mockResolvedValue({ data: { id: 9 } })
    await w.get('[data-testid="composer-submit"]').trigger('click')
    await flushPromises()
    expect(composer.open).toBe(false)
    expect(draft.content).toBe('')
    expect(draft.scenarioKey).toBe('')
  })

  it('提交中关不掉面板，失败原因留在面板里', async () => {
    const w = setup()
    await flushPromises()
    await pick(w, '调用报错')
    await w.get('[data-testid="composer-content"]').setValue('调用 claude 一直返回 502 错误')
    let fail!: (e: unknown) => void
    post.mockReturnValue(new Promise((_, reject) => (fail = reject)))
    await w.get('[data-testid="composer-submit"]').trigger('click')
    expect(w.get('[data-testid="composer-submit"]').text()).toBe('提交中…')

    await w.get('[data-testid="sheet-close"]').trigger('click')
    expect(composer.open).toBe(true)

    fail({ message: '你有 5 条反馈还在处理中' })
    await flushPromises()
    expect(w.get('[data-testid="composer-error"]').text()).toContain('还在处理中')
    await w.get('[data-testid="sheet-close"]').trigger('click')
    expect(composer.open).toBe(false)
  })

  it('已停用的场景不出现在可选列表里', async () => {
    const w = setup({
      scenarios: [
        { key: 'call_error', kind: 'issue', label: '调用报错', icon: '⚠️', hint: '', fields: [] },
        { key: 'old', kind: 'issue', label: '旧场景', icon: '📦', hint: '', fields: [], enabled: false }
      ]
    })
    await flushPromises()
    expect(w.text()).toContain('调用报错')
    expect(w.text()).not.toContain('旧场景')
  })
})

async function pick(w: ReturnType<typeof mount>, label: string) {
  await w.findAll('button').find((b) => b.text().includes(label))!.trigger('click')
}

describe('面板与图片预览的 Esc', () => {
  const esc = () => new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })

  it('焦点不在面板里也能按 Esc 关闭；已被下拉框等处理过的 Esc 不关；打开期间锁定页面滚动', async () => {
    const w = mount(Sheet, { props: { open: true, title: '反馈' }, attachTo: document.body })
    await flushPromises()
    expect(document.body.classList.contains('modal-open')).toBe(true)

    const handled = esc()
    handled.preventDefault()
    document.dispatchEvent(handled)
    expect(w.emitted('close')).toBeUndefined()

    document.dispatchEvent(esc())
    expect(w.emitted('close')).toHaveLength(1)

    await w.setProps({ open: false })
    expect(document.body.classList.contains('modal-open')).toBe(false)
    w.unmount()
  })

  it('面板里的图片预览按 Esc 只关预览，不关面板', async () => {
    const onClose = vi.fn()
    const w = mount(
      { components: { Sheet, AuthImage }, setup: () => ({ onClose }), template: '<Sheet :open="true" title="反馈" @close="onClose"><AuthImage :id="1" /></Sheet>' },
      { attachTo: document.body }
    )
    await flushPromises()
    const thumb = document.body.querySelector<HTMLButtonElement>('button[aria-label="查看图片"]')!
    thumb.click()
    await flushPromises()
    expect(document.body.querySelector('[aria-label="图片预览"]')).not.toBeNull()

    document.activeElement!.dispatchEvent(esc())
    await flushPromises()
    expect(document.body.querySelector('[aria-label="图片预览"]')).toBeNull()
    expect(onClose).not.toHaveBeenCalled()
    expect(document.activeElement).toBe(thumb)
    w.unmount()
  })
})

