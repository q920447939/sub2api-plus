// [plus] 上游调度台：通用格式化。等级、作用、动作的中文名和颜色都在这里统一。
import type { Level, Role, TypeSpec, MetricSpec } from '../api'
export { shortTime, fullTime } from '@/plus/iq/shared/format'

export const LEVEL_TEXT: Record<Level, string> = { ok: '正常', warn: '可疑', bad: '异常', few: '数据不足', none: '未检测', info: '' }
export const LEVEL_CLASS: Record<Level, string> = {
  ok: 'bg-green-50 text-green-700 border-green-200 dark:bg-green-900/20 dark:text-green-300 dark:border-green-800',
  warn: 'bg-amber-50 text-amber-700 border-amber-200 dark:bg-amber-900/20 dark:text-amber-300 dark:border-amber-800',
  bad: 'bg-red-50 text-red-700 border-red-300 font-semibold dark:bg-red-900/20 dark:text-red-300 dark:border-red-800',
  few: 'bg-gray-50 text-gray-500 border-dashed border-gray-300 dark:bg-dark-800 dark:text-dark-300 dark:border-dark-600',
  none: 'bg-transparent text-gray-400 border-gray-200 dark:border-dark-700',
  info: 'bg-gray-100 text-gray-600 border-transparent dark:bg-dark-700 dark:text-dark-200'
}
export const LEVEL_DOT: Record<Level, string> = { ok: '#16a34a', warn: '#f59e0b', bad: '#dc2626', few: '#cbd5e1', none: 'transparent', info: '#94a3b8' }
export const LEVEL_RANK: Record<Level, number> = { bad: 4, warn: 3, ok: 2, few: 1, info: 0.5, none: 0 }

export const ROLE_TEXT: Record<Role, string> = { gate: '门槛', score: '参与调度', display: '仅展示' }
export const ROLE_DESC: Record<Role, string> = {
  gate: '判为异常时减少或停止分配',
  score: '有问题时按系数减少份额',
  display: '不影响流量'
}
export const ROLE_CLASS: Record<Role, string> = {
  gate: 'bg-red-50 text-red-600 dark:bg-red-900/20 dark:text-red-300',
  score: 'bg-blue-50 text-blue-600 dark:bg-blue-900/20 dark:text-blue-300',
  display: 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-300'
}

export const MODE_TEXT = { off: '关闭', observe: '只观察', manual: '人工调整' } as const
export const MODE_DESC = {
  off: '不检测、不判定，调度按原逻辑。',
  observe: '照常检测和给建议，调整不生效。',
  manual: '调整立即生效。'
} as const

export const PLATFORM_TEXT: Record<string, string> = { anthropic: 'Claude', openai: 'OpenAI', gemini: 'Gemini', antigravity: 'Antigravity' }
export const platformName = (p: string) => PLATFORM_TEXT[p] ?? p
export const PLATFORM_CLASS: Record<string, string> = {
  anthropic: 'bg-orange-50 text-orange-700 dark:bg-orange-900/20 dark:text-orange-300',
  openai: 'bg-green-50 text-green-700 dark:bg-green-900/20 dark:text-green-300',
  gemini: 'bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-300'
}

export function adjustText(action?: string, factor?: number): string {
  if (action === 'pause') return '已暂停'
  if (action === 'factor') return `×${factor}`
  return '正常'
}

export const TONE_CLASS: Record<string, string> = {
  ok: 'bg-green-50 text-green-700 border-green-200 dark:bg-green-900/20 dark:text-green-300',
  bad: 'bg-red-50 text-red-700 border-red-200 font-semibold dark:bg-red-900/20 dark:text-red-300',
  few: 'bg-gray-50 text-gray-500 border-gray-200 dark:bg-dark-800 dark:text-dark-300',
  err: 'bg-slate-100 text-slate-600 border-slate-300 dark:bg-dark-700 dark:text-dark-200',
  info: 'bg-gray-100 text-gray-600 border-gray-200'
}

export function outcomeOf(spec: TypeSpec | undefined, key: string) {
  return spec?.outcomes?.find((o) => o.key === key) ?? { key, label: key || '—', tone: 'info' as const }
}

export function fmtMetric(m: Pick<MetricSpec, 'unit'> | undefined, v?: number | null): string {
  if (v == null || !Number.isFinite(v)) return '—'
  if (!m) return String(v)
  if (m.unit === '%') return `${v.toFixed(1)}%`
  if (m.unit === 's') return `${v.toFixed(1)} 秒`
  return `${Math.round(v)} ${m.unit}`
}

export function agoText(iso?: string | null): string {
  if (!iso) return '—'
  const m = Math.max(1, Math.round((Date.now() - new Date(iso).getTime()) / 60000))
  if (m < 60) return `${m} 分钟前`
  if (m < 1440) return `${Math.round(m / 60)} 小时前`
  return `${Math.round(m / 1440)} 天前`
}

export function everyText(min: number): string {
  if (min % 1440 === 0) return `每 ${min / 1440} 天`
  if (min % 60 === 0) return `每 ${min / 60} 小时`
  return `每 ${min} 分钟`
}

export const PALETTE = ['#0d9488', '#6366f1', '#e11d48', '#f59e0b', '#0ea5e9', '#8b5cf6', '#84cc16', '#ec4899', '#14b8a6', '#64748b']
