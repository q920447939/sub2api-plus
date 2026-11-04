import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { AdminConfig, Config } from '../api'

let adminConfig: () => AdminConfig
let runCalls: unknown[] = []

vi.mock('../api', () => ({
  getAdminConfig: () => Promise.resolve(adminConfig()),
  saveAdminConfig: () => Promise.resolve(2),
  listProbeKeys: () =>
    Promise.resolve([
      { id: 11, name: 'my-claude', masked: 'sk-abc****1234', group_id: 1 },
      { id: 12, name: 'my-gpt', masked: 'sk-def****5678', group_id: 2 }
    ]),
  listProbeStates: () => Promise.resolve([]),
  runProbe: (...a: unknown[]) => {
    runCalls.push(a)
    return Promise.resolve({ ok: true, latency_ms: 1200, message: '' })
  }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: () => {}, showError: () => {} }) }))

import { dirty, loadSettings, settings, validateDraft } from '../admin/state'
import ProbeTab from '../admin/ProbeTab.vue'

function baseConfig(): Config {
  return {
    enabled: true,
    auto_show_new_groups: true,
    refresh_seconds: 60,
    judge_window_minutes: 15,
    default_granularity: 'hour',
    layout: [],
    rules: {
      up_min: 80, warn_min: 50, min_sample: 50, history_min_sample: 10,
      warning_error_rate: 0.05, critical_error_rate: 0.2, target_ttft_ms: 3000, critical_ttft_ms: 10000,
      warning_cache_rate: 0, critical_cache_rate: 0, error_weight: 0.6, ttft_weight: 0.2, cache_weight: 0.2,
      ignored_error_categories: []
    },
    probe: {
      enabled: true, endpoint: 'https://api.example.com', fresh_minutes: 30, retry_minutes: 5,
      base_cooldown_minutes: 10, max_cooldown_hours: 6, daily_limit: 24, timeout_seconds: 30, concurrency: 2,
      groups: [{ group_id: 1, enabled: true, api_key_id: 11, model: 'claude-haiku-4-5', format: '' }]
    }
  }
}

beforeEach(() => {
  runCalls = []
  adminConfig = () => ({
    config: baseConfig(),
    version: 1,
    groups: [
      { id: 1, name: 'Claude Max', platform: 'anthropic' },
      { id: 2, name: 'GPT', platform: 'openai' }
    ],
    new_group_ids: [],
    error_categories: []
  })
})

describe('兜底探测设置', () => {
  it('加载时为缺失的分组补默认关闭的探测项，且不算未保存修改', async () => {
    await loadSettings()
    expect(settings.draft!.probe.groups.map((g) => [g.group_id, g.enabled])).toEqual([[1, true], [2, false]])
    expect(dirty.value).toBe(false)
  })

  it('探测配置问题定位到探测 Tab', async () => {
    await loadSettings()
    const c = settings.draft!
    expect(validateDraft(c)).toEqual(['', ''])
    c.probe.groups[1].enabled = true
    expect(validateDraft(c)[0]).toBe('probe')
    c.probe.groups[1].enabled = false
    c.probe.endpoint = ''
    expect(validateDraft(c)[1]).toContain('网关地址')
  })

  it('Key 只列出绑定该分组的，立即探测带上草稿', async () => {
    await loadSettings()
    const w = mount(ProbeTab, { global: { stubs: { Toggle: true } } })
    await flushPromises()
    const rows = w.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    const opts = rows[0].findAll('select')[0].findAll('option').map((o) => o.text())
    expect(opts.some((t) => t.includes('my-claude'))).toBe(true)
    expect(opts.some((t) => t.includes('my-gpt'))).toBe(false)
    expect(rows[1].findAll('select')[1].find('option').text()).toContain('OpenAI Responses')
    await rows[0].find('button').trigger('click')
    await flushPromises()
    expect(runCalls).toHaveLength(1)
    expect((runCalls[0] as [unknown, { group_id: number }])[1].group_id).toBe(1)
  })
})
