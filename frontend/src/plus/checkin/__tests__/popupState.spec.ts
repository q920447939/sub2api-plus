import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('../api', () => ({
  getCheckinStatus: vi.fn(),
  postCheckin: vi.fn()
}))

import {
  dismissForSession,
  isPopupSuppressed,
  muteForToday,
  resetCheckinPopupSession
} from '../popupState'
import { checkinRewardFor, checkinTodayPos } from '../week'

describe('checkin popup suppression', () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
  })

  it('shows by default', () => {
    expect(isPopupSuppressed(1, '2026-10-24')).toBe(false)
  })

  it('"later" hides for this session only', () => {
    dismissForSession(1)
    expect(isPopupSuppressed(1, '2026-10-24')).toBe(true)
    resetCheckinPopupSession() // 退出登录
    expect(isPopupSuppressed(1, '2026-10-24')).toBe(false)
  })

  it('"mute today" survives re-login but expires next day', () => {
    muteForToday(1, '2026-10-24')
    resetCheckinPopupSession()
    expect(isPopupSuppressed(1, '2026-10-24')).toBe(true)
    expect(isPopupSuppressed(1, '2026-10-25')).toBe(false)
  })

  it('is scoped per user', () => {
    muteForToday(1, '2026-10-24')
    expect(isPopupSuppressed(2, '2026-10-24')).toBe(false)
  })
})

describe('checkin week math', () => {
  it('positions today in the 7-day cycle', () => {
    expect(checkinTodayPos({ streak: 0, today_checked: false })).toBe(1)
    expect(checkinTodayPos({ streak: 6, today_checked: false })).toBe(7)
    expect(checkinTodayPos({ streak: 7, today_checked: true })).toBe(7)
    expect(checkinTodayPos({ streak: 7, today_checked: false })).toBe(1)
  })

  it('adds week bonus on day 7', () => {
    const s = { rewards: [1, 2, 3, 4, 5, 6, 7], week_bonus: 50 }
    expect(checkinRewardFor(s, 1)).toBe(1)
    expect(checkinRewardFor(s, 7)).toBe(57)
  })
})
