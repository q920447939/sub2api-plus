// [plus] 上游调度台：通用格式化和配色。状态配色、平台名沿用「渠道状态」，两边看起来一致。
import type { Cols, OutcomeSpec, RuleState, Rule, TypeSpec } from '../api'
export { STATUS_META, platformName } from '@/plus/channelstatus/status'
export { fullTime } from '@/plus/iq/shared/format'

const pad = (n: number) => String(n).padStart(2, '0')

/** 11-12 20:14 */
export function dt(iso?: string | number | null): string {
  if (iso == null || iso === '') return '—'
  const d = typeof iso === 'number' ? new Date(iso * 1000) : new Date(iso)
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** 状态条一格的时间说法 */
export function slotTime(t: number, gran: 'hour' | 'day'): string {
  const d = new Date(t * 1000)
  return gran === 'day' ? `${d.getMonth() + 1}月${d.getDate()}日` : `${d.getMonth() + 1}月${d.getDate()}日 ${pad(d.getHours())}:00`
}

/** 最近请求：今天的写到秒，否则写日期 */
export function lastText(lastAt: string | null, lastHour: number | null): string {
  if (lastAt) {
    const d = new Date(lastAt)
    return d.toDateString() === new Date().toDateString() ? `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}` : dt(lastAt)
  }
  if (lastHour) {
    const d = new Date(lastHour * 1000)
    return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())} 时`
  }
  return '—'
}

/** 1.2 万 / 3,456 */
export function num(n?: number | null): string {
  if (n == null) return '—'
  return n >= 10000 ? `${(n / 10000).toFixed(1)} 万` : Math.round(n).toLocaleString('en-US')
}
export const times = (n?: number | null) => (n == null ? '—' : `${num(n)} 次`)

/** 24.9B / 13.1M */
export function tok(n?: number | null): string {
  if (!n) return '0'
  if (n >= 1e9) return `${(n / 1e9).toFixed(1)}B`
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)}M`
  if (n >= 1e3) return `${(n / 1e3).toFixed(1)}K`
  return String(Math.round(n))
}

export function pct(v?: number | null): string {
  if (v == null) return '—'
  if (v > 0 && v < 1) return '<1%'
  return `${Math.round(v)}%`
}

export function pct1(v?: number | null): string {
  if (v == null) return '—'
  return v < 10 ? `${v.toFixed(1)}%` : `${Math.round(v)}%`
}

export function unitVal(unit: string, v?: number | null): string {
  if (v == null || !Number.isFinite(v)) return '—'
  if (unit === '%') return pct1(v)
  if (unit === 's') return `${v.toFixed(1)} 秒`
  return `${Math.round(v)} ${unit}`
}

export function everyText(min?: number | null): string {
  if (!min) return '手动'
  if (min % 1440 === 0) return `每 ${min / 1440} 天`
  if (min % 60 === 0) return `每 ${min / 60} 小时`
  return `每 ${min} 分钟`
}

// ---------- 数字按好坏着色（和原型一致的分档） ----------
export const LV = {
  score: (v: number | null | undefined) => (v == null ? 'text-gray-400' : v >= 80 ? 'text-emerald-600' : v >= 50 ? 'text-amber-600' : 'text-red-600'),
  err: (v: number | null | undefined) => (v == null ? 'text-gray-400' : v < 5 ? 'text-emerald-600' : v < 20 ? 'text-amber-600' : 'text-red-600'),
  ttft: (v: number | null | undefined) => (v == null ? 'text-gray-400' : v <= 3 ? 'text-emerald-600' : v <= 10 ? 'text-amber-600' : 'text-red-600'),
  cache: (v: number | null | undefined) => (v == null ? 'text-gray-400' : v >= 60 ? 'text-emerald-600' : v >= 30 ? 'text-amber-600' : 'text-red-600')
}

/** 数字列：账号列表、模型列表、对比表共用。每列一个主数字 + 一行小字 */
export const COLS = [
  { key: 'score', label: '健康分', tip: '按最近的失败率和首字耗时算出的综合分，80 以上正常、50 以上降级，算法和渠道状态一样',
    val: (c: Cols) => c.score },
  { key: 'err', label: '24H 错误率', tip: '近 24 小时上游报错、超时、限流的请求占比', val: (c: Cols) => c.err_pct },
  { key: 'ttft', label: '首字耗时', tip: '近 24 小时从发出请求到收到第一个字的时间（中位数）', val: (c: Cols) => c.ttft },
  { key: 'cache', label: '缓存命中', tip: '近 24 小时输入里命中缓存的比例', val: (c: Cols) => c.cache }
] as const

// ---------- 权重 ----------
export function ruleText(r?: RuleState | Rule | null): string {
  if (!r) return '正常'
  if (r.share <= 0) return r.hard ? '暂停（全停）' : '暂停'
  return `${Math.round(r.pct ?? r.share * 100)}%`
}

/** 账号（或它的某个模型）在各分组的设置汇总成按钮上的字 */
export function flowText(rules: Rule[], groups: number, model: string | null): { text: string; tone: '' | 'part' | 'off' } {
  const rs = rules.filter((r) => (r.model || '') === (model || ''))
  if (!rs.length) {
    if (model) return { text: '跟随账号', tone: '' }
    // 账号行：只在某些模型上调过（没有「全部模型」的设置）也要显示出来，不能写「正常」
    const mr = rules.filter((r) => r.model)
    const models = [...new Set(mr.map((r) => r.model))]
    if (!models.length) return { text: '正常', tone: '' }
    if (models.length > 1) return { text: `已调 ${models.length} 个模型`, tone: 'part' }
    const texts = [...new Set(mr.map((r) => ruleText(r)))]
    return { text: `${models[0]} ${texts.length === 1 ? texts[0] : '已调'}`, tone: 'part' }
  }
  if (rs.length >= groups && rs.every((r) => r.share <= 0)) return { text: '暂停', tone: 'off' }
  if (groups === 1) return { text: ruleText(rs[0]), tone: rs[0].share <= 0 ? 'off' : 'part' }
  return { text: `已调 ${rs.length} 个分组`, tone: rs.every((r) => r.share <= 0) ? 'off' : 'part' }
}

export const someNames = (list: string[]) => (list.length <= 2 ? list.join('、') : `${list[0]} 等 ${list.length} 个`)

// ---------- 检测 ----------
export const TONE_CLASS: Record<string, string> = {
  ok: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300',
  bad: 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300',
  few: 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-300',
  err: 'bg-red-50 text-red-600 dark:bg-red-900/20 dark:text-red-300',
  info: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-200'
}
export const DOT_CLASS: Record<string, string> = {
  pass: 'bg-primary-500',
  saved: 'bg-primary-500',
  fail: 'bg-red-500',
  error: 'bg-amber-500',
  ignored: 'bg-gray-400',
  low: 'bg-gray-300 dark:bg-dark-500',
  skipped: 'bg-gray-300 dark:bg-dark-500'
}

export function outcomeOf(spec: TypeSpec | undefined, key: string): OutcomeSpec {
  return spec?.outcomes?.find((o) => o.key === key) ?? { key, label: key || '—', tone: 'info' }
}

export const TRIGGER: Record<string, string> = { cron: '定时', manual: '手动' }

export const PALETTE = ['#2a78d6', '#eb6834', '#1baf7a', '#eda100', '#8b5cf6', '#ec4899', '#0ea5e9', '#64748b']
