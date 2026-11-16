// [plus] 反馈中心的展示文案和样式映射
import type { Kind, Severity, Status } from './api'

export const KIND_LABEL: Record<Kind, string> = { issue: '问题', suggestion: '建议' }

export const STATUS_META: Record<Status, { label: string; tone: string }> = {
  pending: { label: '待处理', tone: 'badge-warning' },
  awaiting: { label: '已回复', tone: 'badge-primary' },
  resolved: { label: '已解决', tone: 'badge-success' },
  adopted: { label: '已采纳', tone: 'badge-purple' },
  declined: { label: '不处理', tone: 'badge-gray' },
  duplicate: { label: '重复', tone: 'badge-gray' }
}

export const SEVERITY_META: Record<Exclude<Severity, ''>, { label: string; dot: string; cell: string }> = {
  critical: { label: '严重', dot: 'bg-red-500', cell: 'bg-red-500' },
  major: { label: '一般', dot: 'bg-orange-400', cell: 'bg-orange-400' },
  minor: { label: '轻微', dot: 'bg-yellow-300', cell: 'bg-yellow-400' },
  invalid: { label: '无效', dot: 'bg-gray-300', cell: 'bg-gray-400' }
}

export const SEVERITIES = ['critical', 'major', 'minor', 'invalid'] as const

/** 结案可选结论（与后端 KindPolicy 一致） */
export const VERDICTS: Record<Kind, Status[]> = {
  issue: ['resolved', 'declined'],
  suggestion: ['adopted', 'declined']
}

export const isOpen = (s: Status) => s === 'pending' || s === 'awaiting'

/** 相对时间：刚刚 / 5 分钟前 / 3 小时前 / 昨天 / 3 天前 / 11-02 */
export function ago(iso: string): string {
  const t = new Date(iso).getTime()
  const s = Math.max(0, (Date.now() - t) / 1000)
  if (s < 60) return '刚刚'
  if (s < 3600) return `${Math.floor(s / 60)} 分钟前`
  if (s < 86400) return `${Math.floor(s / 3600)} 小时前`
  const d = Math.floor(s / 86400)
  if (d === 1) return '昨天'
  if (d < 7) return `${d} 天前`
  const dt = new Date(t)
  const md = `${String(dt.getMonth() + 1).padStart(2, '0')}-${String(dt.getDate()).padStart(2, '0')}`
  return dt.getFullYear() === new Date().getFullYear() ? md : `${dt.getFullYear()}-${md}`
}

export function fullTime(iso: string): string {
  return new Date(iso).toLocaleString('zh-CN', { hour12: false })
}

/** 浏览器和系统：把 User-Agent 转成「Chrome 131 · macOS」这样的可读文字，识别不了返回空 */
export function describeUA(ua?: string): string {
  if (!ua) return ''
  const pick = (rules: [RegExp, string][]) => {
    for (const [re, name] of rules) {
      const m = ua.match(re)
      if (m) return m[1] ? `${name} ${m[1].split(/[._]/)[0]}` : name
    }
    return ''
  }
  const browser = pick([
    [/Edg(?:e|A|iOS)?\/([\d.]+)/, 'Edge'],
    [/OPR\/([\d.]+)/, 'Opera'],
    [/MicroMessenger\/([\d.]+)/, '微信'],
    [/Firefox\/([\d.]+)/, 'Firefox'],
    [/CriOS\/([\d.]+)/, 'Chrome'],
    [/Chrome\/([\d.]+)/, 'Chrome'],
    [/Version\/([\d.]+).*Safari/, 'Safari']
  ])
  const os = pick([
    [/Windows NT/, 'Windows'],
    [/(?:iPhone|iPad).*OS ([\d_]+)/, 'iOS'],
    [/Android ([\d.]+)/, 'Android'],
    [/Mac OS X/, 'macOS'],
    [/Linux/, 'Linux']
  ])
  return [browser, os].filter(Boolean).join(' · ')
}

/**
 * 图片尺寸写在说明文字后面：![说明|200](url) 宽 200px；|200x120 指定宽高；|50% 按卡片宽度的百分比。
 * 不写尺寸时按默认上限显示。
 */
export function parseImageSize(alt: string): { alt: string; style: string } {
  const m = /^(.*?)\s*\|\s*(\d{1,4})(%|(?:x(\d{1,4})))?\s*$/.exec(alt)
  if (!m) return { alt, style: '' }
  const w = Math.min(Number(m[2]), m[3] === '%' ? 100 : 2000)
  if (!w) return { alt: m[1], style: '' }
  if (m[3] === '%') return { alt: m[1], style: `width:${w}%` }
  const h = m[4] ? Math.min(Number(m[4]), 2000) : 0
  return { alt: m[1], style: `width:${w}px${h ? `;height:${h}px;object-fit:cover` : ''}` }
}
