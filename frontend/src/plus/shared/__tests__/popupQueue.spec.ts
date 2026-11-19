import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const busy = { value: false }
vi.mock('../waitForQuietScreen', () => ({
  // 模拟：屏幕忙时一直等，安静后返回 true
  waitForQuietScreen: () =>
    new Promise<boolean>((resolve) => {
      const tick = () => (busy.value ? setTimeout(tick, 10) : resolve(true))
      setTimeout(tick, 10)
    })
}))

import { _resetPopupQueue, acquirePopupSlot, POPUP_PRIORITY } from '../popupQueue'

const flush = () => vi.advanceTimersByTimeAsync(2000)

describe('popupQueue', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    busy.value = false
    _resetPopupQueue()
  })
  afterEach(() => vi.useRealTimers())

  it('只放行一个弹窗，归还后再放行下一个', async () => {
    const order: string[] = []
    const a = acquirePopupSlot('checkin', POPUP_PRIORITY.engagement).then((r) => (order.push('checkin'), r))
    const b = acquirePopupSlot('email-verify', POPUP_PRIORITY.warning).then((r) => (order.push('email-verify'), r))
    await flush()
    expect(order).toEqual(['email-verify']) // 警告优先，签到还在排队
    const release = await b
    release!()
    await flush()
    expect(order).toEqual(['email-verify', 'checkin'])
    expect(await a).toBeTypeOf('function')
  })

  it('屏幕忙（如公告弹窗）时不放行', async () => {
    busy.value = true
    let got = false
    void acquirePopupSlot('checkin', POPUP_PRIORITY.engagement).then(() => (got = true))
    await flush()
    expect(got).toBe(false)
    busy.value = false
    await flush()
    expect(got).toBe(true)
  })

  it('取消的申请返回 null 且不占位', async () => {
    const ctl = new AbortController()
    const a = acquirePopupSlot('checkin', POPUP_PRIORITY.engagement, ctl.signal)
    ctl.abort()
    expect(await a).toBeNull()
    const b = acquirePopupSlot('email-verify', POPUP_PRIORITY.warning)
    await flush()
    expect(await b).toBeTypeOf('function')
  })
})
