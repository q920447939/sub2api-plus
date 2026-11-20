import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ChannelStatusView from '../ChannelStatusView.vue'
import type { Snapshot } from '../api'

const getSnapshot = vi.fn()
// 用普通函数转发：失败场景直接返回 rejected promise，不经过 vi.fn 的结果追踪
let failWith: unknown = null
vi.mock('../api', () => ({
  getSnapshot: (...a: unknown[]) => (failWith ? Promise.reject(failWith) : getSnapshot(...a))
}))

const bucket = (status: 'up' | 'unknown') => ({ t: '2026-11-04T10:00:00+08:00', score: status === 'up' ? 97 : null, status })

const LONG_NAME = 'cursor反代cc   备用 缓存有点炸，优先用另外一个分组，这里只是兜底使用'

const snap: Snapshot = {
  granularity: 'hour',
  refresh_seconds: 60,
  platforms: [
    {
      platform: 'anthropic',
      name: 'Claude',
      groups: [
        { group_id: 1, name: 'Claude Max', subtitle: '官方直连', rate_multiplier: 1.4, status: 'up', score: 97, history: [bucket('up'), bucket('unknown')] },
        { group_id: 2, name: LONG_NAME, subtitle: '', rate_multiplier: 1, status: 'unknown', score: null, history: [bucket('unknown'), bucket('unknown')] }
      ]
    },
    {
      platform: 'openai',
      name: 'OpenAI',
      groups: [{ group_id: 3, name: 'GPT', subtitle: '', rate_multiplier: 0.08, status: 'down', score: 12, history: [bucket('up'), bucket('up')] }]
    }
  ]
}

const mountView = () => mount(ChannelStatusView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } } })

describe('ChannelStatusView', () => {
  beforeEach(() => {
    getSnapshot.mockReset()
    failWith = null
  })

  it('渲染第一个平台的分组', async () => {
    getSnapshot.mockResolvedValue(snap)
    const w = mountView()
    await flushPromises()

    const rows = w.findAll('[data-testid="cs-group-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('Claude Max')
    expect(rows[0].text()).toContain('正常')
    expect(rows[0].text()).toContain('97')
    expect(rows[1].text()).toContain('暂无数据')
    expect(rows[1].text()).toContain('--')
    expect(rows[0].findAll('[data-testid="cs-history"] > div')).toHaveLength(2)
    // 不展示顶部汇总统计
    expect(w.text()).not.toContain('部分异常')
    expect(w.text()).not.toContain('平均健康分')
    w.unmount()
  })

  it('Tab 显示平台 Logo，不再显示汇总状态圆点', async () => {
    getSnapshot.mockResolvedValue(snap)
    const w = mountView()
    await flushPromises()
    const tabs = w.findAll('[data-testid="cs-tabs"] button')
    expect(tabs).toHaveLength(2)
    for (const tab of tabs) {
      expect(tab.find('svg').exists()).toBe(true)
      expect(tab.find('.rounded-full.h-2').exists()).toBe(false)
    }
    // OpenAI 下有一个「异常」分组，Tab 上也不再标红
    expect(tabs[1].html()).not.toContain('bg-red-500')
    expect(tabs[0].text()).toContain('Claude')
    expect(tabs[0].text()).toContain('2')
    w.unmount()
  })

  it('每个分组显示基础倍率，长名称完整显示不截断', async () => {
    getSnapshot.mockResolvedValue(snap)
    const w = mountView()
    await flushPromises()
    const rows = w.findAll('[data-testid="cs-group-row"]')
    expect(rows[0].find('[data-testid="cs-group-rate"]').text()).toBe('1.40x')
    expect(rows[1].find('[data-testid="cs-group-rate"]').text()).toBe('1.00x')
    const name = rows[1].find('[data-testid="cs-group-name"]')
    expect(name.text()).toBe(LONG_NAME)
    expect(name.classes()).not.toContain('truncate')

    await w.findAll('[data-testid="cs-tabs"] button')[1].trigger('click')
    expect(w.find('[data-testid="cs-group-rate"]').text()).toBe('0.08x')
    w.unmount()
  })

  it('切换 Tab 显示对应平台的分组', async () => {
    getSnapshot.mockResolvedValue(snap)
    const w = mountView()
    await flushPromises()
    await w.findAll('[data-testid="cs-tabs"] button')[1].trigger('click')
    const openai = w.findAll('[data-testid="cs-group-row"]')
    expect(openai).toHaveLength(1)
    expect(openai[0].text()).toContain('异常')
    w.unmount()
  })

  it('悬停历史格显示时段、状态与健康分', async () => {
    getSnapshot.mockResolvedValue(snap)
    const w = mountView()
    await flushPromises()
    const row = w.findAll('[data-testid="cs-group-row"]')[0]
    expect(row.find('[data-testid="cs-history-tip"]').exists()).toBe(false)
    await row.findAll('[data-testid="cs-history"] > div')[0].trigger('pointerover')
    const tip = row.find('[data-testid="cs-history-tip"]')
    expect(tip.text()).toMatch(/\d{2}-\d{2} \d{2}:00 – \d{2}:00/)
    expect(tip.text()).toContain('正常')
    expect(tip.text()).toContain('97')
    await row.findAll('[data-testid="cs-history"] > div')[1].trigger('pointerover')
    expect(row.find('[data-testid="cs-history-tip"]').text()).toContain('进行中')
    expect(row.find('[data-testid="cs-history-tip"]').text()).toContain('请求过少')
    await row.find('[data-testid="cs-history"]').element.parentElement!.dispatchEvent(new Event('pointerleave'))
    await w.vm.$nextTick()
    expect(row.find('[data-testid="cs-history-tip"]').exists()).toBe(false)
    w.unmount()
  })

  it('切换粒度会带参数重新请求', async () => {
    getSnapshot.mockResolvedValue(snap)
    const w = mountView()
    await flushPromises()
    const dayBtn = w.findAll('button').find((b) => b.text() === '按天')!
    await dayBtn.trigger('click')
    await flushPromises()
    expect(getSnapshot).toHaveBeenLastCalledWith('day')
    w.unmount()
  })

  it('功能关闭（404）时显示未开放', async () => {
    failWith = { status: 404 }
    const w = mountView()
    await flushPromises()
    expect(w.text()).toContain('渠道状态暂未开放')
    w.unmount()
  })
})
