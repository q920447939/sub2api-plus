// [plus] 渠道状态的展示常量：状态文案/配色、平台默认名、错误分类中文名、时间格式
import type { Granularity, Status } from './api'

export const STATUS_META: Record<Status, { label: string; bar: string; dot: string; badge: string }> = {
  up: {
    label: '正常',
    bar: 'bg-primary-500',
    dot: 'bg-primary-500',
    badge: 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
  },
  warn: {
    label: '降级',
    bar: 'bg-amber-500',
    dot: 'bg-amber-500',
    badge: 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  },
  down: {
    label: '异常',
    bar: 'bg-red-500',
    dot: 'bg-red-500',
    badge: 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  },
  unknown: {
    label: '暂无数据',
    bar: 'bg-gray-200 dark:bg-dark-600',
    dot: 'bg-gray-300 dark:bg-dark-500',
    badge: 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-300'
  }
}

export const STATUS_ORDER: Status[] = ['up', 'warn', 'down', 'unknown']

export const GRANULARITIES: { value: Granularity; label: string; span: string }[] = [
  { value: 'hour', label: '按小时', span: '最近 48 小时' },
  { value: 'day', label: '按天', span: '最近 30 天' }
]

// 与后端 layout.go 保持一致
const PLATFORM_NAMES: Record<string, string> = {
  anthropic: 'Claude',
  openai: 'OpenAI',
  gemini: 'Gemini',
  antigravity: 'Antigravity',
  grok: 'Grok',
  kiro: 'Kiro',
  kimi: 'Kimi',
  zhipu: 'GLM',
  deepseek: 'DeepSeek',
  composite: '混合'
}

export function platformName(platform: string): string {
  return PLATFORM_NAMES[platform] ?? platform
}

export const ERROR_CATEGORY_LABELS: Record<string, string> = {
  content_policy: '内容安全拦截',
  authentication: '鉴权失败',
  context_limit: '上下文超限',
  invalid_request: '请求参数错误',
  model_unsupported: '模型不支持',
  group_access: '分组无权限',
  quota_or_balance: '额度/余额不足',
  account_pool_unavailable: '账号池不可用',
  rate_or_capacity: '限流/容量不足',
  timeout: '超时',
  transport_or_stream: '网络/流中断',
  upstream_forbidden: '上游拒绝 (403)',
  not_found: '资源不存在 (404)',
  client_cancelled: '客户端取消',
  upstream_5xx: '上游 5xx',
  internal: '内部错误',
  other: '其他'
}

const pad = (n: number) => String(n).padStart(2, '0')

export function formatBucket(iso: string, g: Granularity): string {
  const d = new Date(iso)
  const md = `${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
  if (g === 'hour') return `${md} ${pad(d.getHours())}:00`
  if (g === 'week') return `${md} 起一周`
  return md
}
