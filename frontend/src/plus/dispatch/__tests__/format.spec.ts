import { describe, expect, it } from 'vitest'
import type { Rule } from '../api'
import { flowText } from '../shared/format'

const rule = (p: Partial<Rule>): Rule => ({ account_id: 1, group_id: 1, model: '', share: 0.5, hard: false, created_at: '', ...p })

describe('flowText', () => {
  it('账号行：只在某个模型上调过，不能写「正常」', () => {
    const rs = [3, 6, 11].map((g) => rule({ group_id: g, model: 'gpt-6.1-sol', share: 0.91, pct: 91 }))
    expect(flowText(rs, 5, null)).toEqual({ text: 'gpt-6.1-sol 91%', tone: 'part' })
  })
  it('账号行：调了几个模型', () => {
    const rs = [rule({ model: 'a', share: 0 }), rule({ model: 'b', share: 0.3 })]
    expect(flowText(rs, 2, null)).toEqual({ text: '已调 2 个模型', tone: 'part' })
  })
  it('同一个模型在各分组设置不一样', () => {
    const rs = [rule({ group_id: 1, model: 'a', share: 0.9, pct: 90 }), rule({ group_id: 2, model: 'a', share: 0.91, pct: 91 })]
    expect(flowText(rs, 2, null).text).toBe('a 已调')
  })
  it('没有任何设置还是「正常」，全部模型的设置优先显示', () => {
    expect(flowText([], 2, null)).toEqual({ text: '正常', tone: '' })
    expect(flowText([rule({ share: 0.2, pct: 20 }), rule({ model: 'a', share: 0 })], 1, null).text).toBe('20%')
  })
})
