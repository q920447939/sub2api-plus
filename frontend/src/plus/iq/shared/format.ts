// [plus] 智商检测：时间格式化（用户页与管理页共用）

const pad = (n: number) => String(n).padStart(2, '0')

function dayStart(d: Date): number {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
}

/** 今天显示 HH:mm，昨天显示「昨天 HH:mm」，更早显示 M/D HH:mm */
export function shortTime(iso: string, now: Date = new Date()): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`
  const days = Math.round((dayStart(now) - dayStart(d)) / 86_400_000)
  if (days === 0) return hm
  if (days === 1) return `昨天 ${hm}`
  return `${d.getMonth() + 1}/${d.getDate()} ${hm}`
}

export function fullTime(iso?: string | null): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

/** 距离现在多少分钟后（已过期显示「即将」） */
export function untilText(iso: string, now: Date = new Date()): string {
  const mins = Math.round((new Date(iso).getTime() - now.getTime()) / 60_000)
  if (mins <= 0) return '即将'
  if (mins < 60) return `${mins} 分钟后`
  const h = Math.floor(mins / 60)
  return mins % 60 ? `${h} 小时 ${mins % 60} 分后` : `${h} 小时后`
}

export function seconds(ms?: number | null): string {
  return ms == null ? '—' : `${Math.round(ms / 1000)}s`
}

export function tokens(n?: number | null): string {
  if (n == null) return '—'
  if (n >= 10_000) return `${(n / 10_000).toFixed(1)} 万`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return String(n)
}

export const PLATFORM_BADGE: Record<string, string> = {
  anthropic: 'bg-orange-50 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
  openai: 'bg-green-50 text-green-700 dark:bg-green-900/30 dark:text-green-300',
  gemini: 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
}

export function platformBadge(p: string): string {
  return PLATFORM_BADGE[p] ?? 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300'
}
