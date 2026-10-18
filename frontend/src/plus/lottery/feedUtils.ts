// [plus] 中奖动态 / 手气榜的展示辅助函数

// 头像底色：按昵称哈希取色，同一昵称颜色稳定
const AVATAR_COLORS = [
  'bg-orange-100 text-orange-700 dark:bg-orange-900/40 dark:text-orange-300',
  'bg-rose-100 text-rose-700 dark:bg-rose-900/40 dark:text-rose-300',
  'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300',
  'bg-sky-100 text-sky-700 dark:bg-sky-900/40 dark:text-sky-300',
  'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
  'bg-violet-100 text-violet-700 dark:bg-violet-900/40 dark:text-violet-300'
]

export function avatarClass(name: string): string {
  let h = 0
  for (const ch of name) h = (h * 31 + ch.charCodeAt(0)) >>> 0
  return AVATAR_COLORS[h % AVATAR_COLORS.length]
}

export function avatarText(name: string): string {
  return (name[0] ?? '?').toUpperCase()
}

export function relativeTime(iso: string, now: number): string {
  const diff = Math.max(0, now - new Date(iso).getTime())
  const m = Math.floor(diff / 60_000)
  if (m < 1) return '刚刚'
  if (m < 60) return `${m} 分钟前`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h} 小时前`
  return `${Math.floor(h / 24)} 天前`
}

/** 大奖门槛：宣传最高可得的 20%，至少 $5 */
export function bigWinThreshold(promoMax: number): number {
  return Math.max(5, promoMax * 0.2)
}
