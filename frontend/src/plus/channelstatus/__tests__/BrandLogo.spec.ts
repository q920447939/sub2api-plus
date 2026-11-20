import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BrandLogo from '../components/BrandLogo.vue'

const brandOf = (platform: string, name = '') => mount(BrandLogo, { props: { platform, name } }).attributes('data-brand')

describe('BrandLogo', () => {
  it('按平台给出彩色品牌 Logo', () => {
    expect(brandOf('anthropic')).toBe('claude')
    expect(brandOf('openai')).toBe('gpt')
    expect(brandOf('gemini')).toBe('gemini')
    expect(brandOf('grok')).toBe('grok')
    expect(brandOf('deepseek')).toBe('deepseek')
  })

  it('名称里的模型关键词优先于平台（openai 平台下的 deepseek 分组显示 DeepSeek）', () => {
    expect(brandOf('openai', 'deepseek')).toBe('deepseek')
    expect(brandOf('openai', 'GLM特价')).toBe('glm')
    expect(brandOf('openai', 'kimi特价')).toBe('kimi')
    expect(brandOf('anthropic', 'cursor反代cc')).toBe('claude')
  })

  it('品牌 Logo 带颜色；不认识的平台回退到通用图标', () => {
    const w = mount(BrandLogo, { props: { platform: 'anthropic' } })
    expect(w.find('path').attributes('fill')).toMatch(/^#/)
    expect(mount(BrandLogo, { props: { platform: 'composite' } }).attributes('data-brand')).toBe('none')
  })
})
