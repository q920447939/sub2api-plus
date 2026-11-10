// [plus] 把判定规则写成一句人话（不出现 pp、ratio 之类的术语）
import type { Item, Meta } from '../api'

const unitOf = (u?: string) => (u === '%' ? '%' : u === 's' ? ' 秒' : u ? ' ' + u : '')

export function ruleSentence(item: Item, meta: Meta | null): string {
  const r = item.rule ?? {}
  if (item.type === 'traffic') {
    const m = meta?.metrics.find((x) => x.key === item.params?.metric)
    const refs = (r.bases ?? []).map((b: string) => (b === 'fixed' ? `固定值 ${r.fixed}${unitOf(m?.unit)}` : meta?.references[b] ?? b)).join(' → ')
    let thr = ''
    if (r.mode === 'ratio') thr = m?.better === 'high' ? `参照是它的 ${r.warn} 倍以上记可疑，${r.bad} 倍以上记异常` : `是参照的 ${r.warn} 倍以上记可疑，${r.bad} 倍以上记异常`
    else if (r.mode === 'pct') thr = `只有参照的 ${r.warn}% 以下记可疑，${r.bad}% 以下记异常`
    else thr = m?.better === 'high' ? `比参照低 ${r.warn} 个百分点记可疑，低 ${r.bad} 个记异常` : `比参照高 ${r.warn} 个百分点记可疑，高 ${r.bad} 个记异常`
    const floor = r.floor != null && m?.better === 'low' ? `；低于 ${r.floor}${unitOf(m?.unit)} 的一律算正常` : ''
    const few = m?.counted ? `；出问题的请求少于 ${r.min_events ?? 3} 次不算` : ''
    return `和参照比（按顺序用第一个可用的：${refs}；可比的少于 ${r.min_peers} 家就换下一个），${thr}${floor}${few}，持续 ${r.duration_minutes} 分钟以上才算；近 1 小时请求少于 ${r.min_samples} 次不下结论`
  }
  if (item.type === 'fingerprint') return `连续 ${r.bad_consecutive} 次识别成别的模型记异常，1 次起记可疑；置信度低于 ${r.min_confidence}% 的结果不计入；请求出错不计入`
  if (item.type === 'graded') {
    if (r.how === 'peer') return `近 ${r.window} 次的答对次数比同组同模型其他账号的平均少 ${r.peer_warn} 次记可疑，少 ${r.peer_bad} 次记异常（没有可比的账号时改用：少于 ${r.warn_below} 次可疑、少于 ${r.bad_below} 次异常）；没攒满 ${r.window} 次时按比例换算；请求出错不计入`
    return `近 ${r.window} 次里答对少于 ${r.warn_below} 次记可疑，少于 ${r.bad_below} 次记异常；没攒满 ${r.window} 次时按比例换算；请求出错不计入`
  }
  return '不判定，只展示'
}
