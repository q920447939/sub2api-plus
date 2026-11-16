import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('@/api/client', () => ({ apiClient: { get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn() } }))

import { describeUA } from '../format'
import MarkdownView from '../components/MarkdownView.vue'
import Segmented from '../components/Segmented.vue'

describe('describeUA', () => {
  it('转成可读的浏览器和系统', () => {
    expect(describeUA('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36')).toBe('Chrome 131 · macOS')
    expect(describeUA('Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/130.0 Safari/537.36 Edg/130.0.1')).toBe('Edge 130 · Windows')
    expect(describeUA('Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 Version/17.5 Mobile/15E148 Safari/604.1')).toBe('Safari 17 · iOS 17')
    expect(describeUA('')).toBe('')
  })
})

describe('MarkdownView', () => {
  it('渲染 Markdown，但原始 HTML 一律转义', () => {
    const w = mount(MarkdownView, { props: { source: '**紧急**请进群\n\n<img src=x onerror=alert(1)><script>alert(2)</script>' } })
    const html = w.html()
    expect(html).toContain('<strong>紧急</strong>')
    expect(html).not.toContain('<img')
    expect(html).not.toContain('<script')
  })

  it('链接新窗口打开，危险协议被替换', () => {
    const w = mount(MarkdownView, { props: { source: '[群](https://t.me/x) [坏](javascript:alert(1))' } })
    const links = w.findAll('a')
    expect(links[0].attributes('href')).toBe('https://t.me/x')
    expect(links[0].attributes('target')).toBe('_blank')
    expect(links[1].attributes('href')).toBe('#')
  })
})

describe('Segmented', () => {
  it('点击切换并显示计数', async () => {
    const w = mount(Segmented, {
      props: { modelValue: 'a', options: [{ value: 'a', label: 'A' }, { value: 'b', label: 'B', count: 3 }] }
    })
    expect(w.text()).toContain('3')
    await w.findAll('button')[1].trigger('click')
    expect(w.emitted('update:modelValue')?.[0]).toEqual(['b'])
  })
})
