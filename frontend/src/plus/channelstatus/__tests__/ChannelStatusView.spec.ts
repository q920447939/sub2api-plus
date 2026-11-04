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

const snap: Snapshot = {
  granularity: 'hour',
  refresh_seconds: 60,
  platforms: [
    {
      platform: 'anthropic',
      name: 'Claude',
      groups: [
        { group_id: 1, name: 'Claude Max', subtitle: '官方直连', status: 'up', score: 97, history: [bucket('up'), bucket('unknown')] },
        { group_id: 2, name: 'Claude 特惠', subtitle: '', status: 'unknown', score: null, history: [bucket('unknown'), bucket('unknown')] }
      ]
    },
    {
      platform: 'openai',
      name: 'OpenAI',
      groups: [{ group_id: 3, name: 'GPT', subtitle: '', status: 'down', score: 12, history: [bucket('up'), bucket('up')] }]
    }
  ]
}

const mountView = () => mount(ChannelStatusView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } } })

describe('ChannelStatusView', () => {
  beforeEach(() => {
    getSnapshot.mockReset()
    failWith = null
  })

  it('渲染第一个平台的分组，并可切换 Tab', async () => {
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

    await w.findAll('[data-testid="cs-tabs"] button')[1].trigger('click')
    const openai = w.findAll('[data-testid="cs-group-row"]')
    expect(openai).toHaveLength(1)
    expect(openai[0].text()).toContain('异常')
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
