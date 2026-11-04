// [plus] 渠道状态设置的草稿状态：各 Tab 只读写这里，保存/重置统一由页面处理
import { computed, reactive } from 'vue'
import { getAdminConfig, saveAdminConfig, type Config, type GroupInfo } from '../api'

interface State {
  loaded: boolean
  saving: boolean
  version: number
  draft: Config | null
  original: string // 上次加载/保存的 JSON，用于脏检查与重置
  groups: Record<number, GroupInfo>
  newGroupIds: Set<number>
  errorCategories: string[]
}

export const settings = reactive<State>({
  loaded: false,
  saving: false,
  version: 0,
  draft: null,
  original: '',
  groups: {},
  newGroupIds: new Set(),
  errorCategories: []
})

export const dirty = computed(() => !!settings.draft && JSON.stringify(settings.draft) !== settings.original)

export async function loadSettings(): Promise<void> {
  const res = await getAdminConfig()
  settings.version = res.version
  settings.groups = Object.fromEntries(res.groups.map((g) => [g.id, g]))
  settings.newGroupIds = new Set(res.new_group_ids)
  settings.errorCategories = res.error_categories
  // 每个分组都补一条（默认关闭的）探测项，放在记录原始快照之前，避免一打开就显示"有未保存的修改"
  const probed = new Set(res.config.probe.groups.map((g) => g.group_id))
  for (const g of res.groups) {
    if (!probed.has(g.id)) res.config.probe.groups.push({ group_id: g.id, enabled: false, api_key_id: 0, model: '', format: '' })
  }
  settings.original = JSON.stringify(res.config)
  settings.draft = res.config
  settings.loaded = true
}

export function resetSettings(): void {
  if (settings.original) settings.draft = JSON.parse(settings.original)
}

export async function saveSettings(): Promise<void> {
  if (!settings.draft) return
  settings.saving = true
  try {
    settings.version = await saveAdminConfig(settings.draft, settings.version)
    settings.original = JSON.stringify(settings.draft)
    settings.newGroupIds = new Set()
  } finally {
    settings.saving = false
  }
}

// 前端预校验，与后端 Config.Validate 保持一致；返回 [所在 Tab, 第一条问题]
export function validateDraft(c: Config): [string, string] {
  const rules = validateRules(c)
  if (rules) return ['rules', rules]
  const probe = validateProbe(c)
  if (probe) return ['probe', probe]
  return ['', '']
}

function validateProbe(c: Config): string {
  const p = c.probe
  const ranges: [number, number, number, string][] = [
    [p.fresh_minutes, 5, 720, '探测结果保质期'],
    [p.retry_minutes, 1, 120, '失败复测间隔'],
    [p.base_cooldown_minutes, 1, 720, '冷却起点'],
    [p.max_cooldown_hours, 1, 48, '冷却上限'],
    [p.daily_limit, 1, 1440, '每日探测上限'],
    [p.timeout_seconds, 5, 120, '单次探测超时'],
    [p.concurrency, 1, 16, '并发上限']
  ]
  for (const [v, lo, hi, name] of ranges) {
    if (!(Number.isInteger(v) && v >= lo && v <= hi)) return `${name}需在 ${lo}-${hi} 之间`
  }
  if (p.base_cooldown_minutes > p.max_cooldown_hours * 60) return '冷却起点不能大于冷却上限'
  const enabled = p.groups.filter((g) => g.enabled)
  const bad = enabled.find((g) => !(g.api_key_id > 0) || !g.model.trim())
  if (bad) return `已启用探测的分组「${settings.groups[bad.group_id]?.name ?? bad.group_id}」需选择 Key 和模型`
  if (p.enabled && enabled.length > 0 && !p.endpoint.trim()) return '启用探测需填写网关地址'
  if (p.endpoint.trim() && !/^https?:\/\/[^/\s]+/.test(p.endpoint.trim())) return '网关地址格式不正确'
  return ''
}

function validateRules(c: Config): string {
  const r = c.rules
  if (!(r.warn_min > 0 && r.warn_min < r.up_min && r.up_min <= 100)) return '分档需满足 0 < 降级线 < 正常线 ≤ 100'
  if (!(r.min_sample >= 1 && r.min_sample <= 10000)) return '最小样本需在 1-10000 之间'
  if (!(r.history_min_sample >= 1 && r.history_min_sample <= 10000)) return '历史条最小样本需在 1-10000 之间'
  if (!(r.warning_error_rate > 0 && r.critical_error_rate >= r.warning_error_rate && r.critical_error_rate <= 1))
    return '错误率阈值需满足 0 < 告警 ≤ 严重 ≤ 100%'
  if (!(r.target_ttft_ms > 0 && r.critical_ttft_ms > r.target_ttft_ms)) return 'TTFT 需满足 0 < 目标 < 严重'
  if (!(r.critical_cache_rate >= 0 && r.critical_cache_rate <= r.warning_cache_rate && r.warning_cache_rate <= 1))
    return '缓存命中率阈值需满足 0 ≤ 严重 ≤ 告警 ≤ 100%'
  if ([r.error_weight, r.ttft_weight, r.cache_weight].some((w) => w < 0)) return '权重不能为负'
  if (Math.abs(r.error_weight + r.ttft_weight + r.cache_weight - 1) > 0.001) return '三项权重之和必须等于 100%'
  return ''
}
