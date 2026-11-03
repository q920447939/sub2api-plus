/**
 * [plus] 积分乐园管理端共享状态：签到 / 幸运九宫格 / 积分三份配置。
 * 三份配置互相引用（连签加成要选九宫格奖档、奖金罐预览要读连签加成），所以统一加载；
 * 各管理页只保存自己那一份。各配置组件直接引用这里的响应式对象，避免层层传 props。
 */
import { reactive } from 'vue'
import { adminGetCheckinConfig, adminSaveCheckinConfig, type CheckinConfig } from '../checkin/api'
import { adminGetPointsConfig, adminSavePointsConfig, type PointsConfig } from '../points/api'
import { adminGetSlotState, adminSaveSlotConfig, type SlotAdminState, type SlotTier } from '../slot/api'

export const FULL_BP = 10000

export const park = reactive({
  loaded: false,
  checkin: null as CheckinConfig | null,
  checkinVer: 0,
  points: null as PointsConfig | null,
  pointsVer: 0,
  slot: null as SlotAdminState | null
})

export async function loadParkAdmin(): Promise<void> {
  const [ci, pi, si] = await Promise.all([adminGetCheckinConfig(), adminGetPointsConfig(), adminGetSlotState()])
  park.checkin = ci.config
  park.checkinVer = ci.version
  park.points = pi.config
  park.pointsVer = pi.version
  park.slot = si
  park.loaded = true
}

/** 只刷新九宫格的运行数据（奖金罐余额、统计），不覆盖正在编辑的配置 */
export async function refreshSlotStats(): Promise<void> {
  const si = await adminGetSlotState()
  if (!park.slot) return
  park.slot.pots = si.pots
  park.slot.spins = si.spins
}

export type ParkSection = 'checkin' | 'slot' | 'points'

/** 只保存某个模块的配置，保存后重新拉取（服务端会规整字段并重算奖金罐预览） */
export async function saveParkSection(section: ParkSection): Promise<void> {
  if (section === 'checkin' && park.checkin) {
    park.checkinVer = await adminSaveCheckinConfig(park.checkin, park.checkinVer)
  } else if (section === 'points' && park.points) {
    park.pointsVer = await adminSavePointsConfig(park.points, park.pointsVer)
  } else if (section === 'slot' && park.slot) {
    park.slot.version = await adminSaveSlotConfig(park.slot.config, park.slot.version)
  }
  await loadParkAdmin()
}

/** 前端先拦明显错误，详细校验以后端为准 */
export function sectionProblems(section: ParkSection): string[] {
  const out: string[] = []
  if (section === 'slot' && park.slot) {
    const slot = park.slot.config
    const sum = slot.tiers.reduce((s, t) => s + (t.prob_bp || 0), 0)
    if (sum !== FULL_BP) out.push('奖品概率合计必须为 100%')
    if (slot.tiers.filter((t) => t.prize_type === 'none').length !== 1) out.push('需要且只能有 1 个“谢谢惠顾”')
    if ((slot.pot.small_pct || 0) + (slot.pot.jackpot_pct || 0) > 100) out.push('奖金罐注入比例合计不能超过 100%')
    const v = slot.atmosphere.virtual_probs.reduce((s, x) => s + (x || 0), 0)
    if (v !== FULL_BP) out.push('人气氛围：虚拟记录奖品分布合计必须为 100%')
  }
  if (section === 'checkin') {
    const sb = park.checkin?.streak_boost
    if (sb?.enabled && !sb.tiers.length) out.push('开启连签加成时至少选择一个奖品')
  }
  return out
}

/** 九宫格奖档（展示用），未加载时为空 */
export function gridTiers(): SlotTier[] {
  return park.slot?.config.tiers ?? []
}

export function tierName(i: number): string {
  const t = gridTiers()[i]
  return t ? `${t.emoji ?? ''} ${t.label}`.trim() : `第 ${i + 1} 档`
}

/** 万分比 ↔ 百分比输入框 */
export const bpToPct = (bp: number) => Math.round(bp) / 100
export const pctToBp = (v: string | number) => {
  const n = Number(v)
  return Number.isFinite(n) ? Math.round(n * 100) : 0
}
