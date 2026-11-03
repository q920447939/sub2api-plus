// [plus] 7 天循环的计算，签到页、弹窗、单测共用
import type { CheckinStatus } from './api'

/** 今天在 7 天循环里的位置（1-7）：已签取 streak，未签取 streak+1 */
export function checkinTodayPos(s: Pick<CheckinStatus, 'streak' | 'today_checked'>): number {
  const n = s.today_checked ? s.streak : s.streak + 1
  return ((Math.max(n, 1) - 1) % 7) + 1
}

/** 第 d 天的奖励，第 7 天叠加满签奖励 */
export function checkinRewardFor(s: Pick<CheckinStatus, 'rewards' | 'week_bonus'>, d: number): number {
  return (s.rewards[d - 1] ?? 0) + (d === 7 ? s.week_bonus : 0)
}
