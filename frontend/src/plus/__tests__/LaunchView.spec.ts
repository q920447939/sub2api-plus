import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
import LaunchView from '../LaunchView.vue'
import { clearFrames, embedHostState, getFrame } from '../embed/embedHost'

const launchMenu = vi.fn()
vi.mock('@/plus/api', () => ({ launchMenu: (...a: unknown[]) => launchMenu(...a) }))

const route = reactive({ params: { id: 'img' } as Record<string, string> })
vi.mock('vue-router', () => ({ useRoute: () => route }))
vi.mock('vue-i18n', async () => ({
  ...(await vi.importActual<typeof import('vue-i18n')>('vue-i18n')),
  useI18n: () => ({ t: (key: string) => key }),
}))

const menus = [
  { id: 'img', label: '文生图', url: 'plus:key_launch', open_mode: 'embed', visibility: 'user', sort_order: 0 },
  { id: 'img2', label: '图生图', url: 'plus:key_launch', open_mode: 'embed', visibility: 'user', sort_order: 1, hide_open_button: true },
  { id: 'tab', label: '新窗口', url: 'plus:key_launch', open_mode: 'new_tab', visibility: 'user', sort_order: 2 },
]
const authStore = reactive({ user: { id: 7 } as { id: number } | null, isAdmin: false })
vi.mock('@/stores', () => ({
  useAppStore: () => ({ cachedPublicSettings: { custom_menu_items: menus } }),
  useAuthStore: () => authStore,
  useAdminSettingsStore: () => ({ customMenuItems: [] }),
}))

const mountView = () =>
  mount(LaunchView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } } })

describe('LaunchView', () => {
  beforeEach(() => {
    clearFrames()
    launchMenu.mockReset()
    route.params.id = 'img'
    authStore.user = { id: 7 }
  })

  it('embed 模式把地址交给常驻层，自己只留占位', async () => {
    launchMenu.mockResolvedValue({ kind: 'ok', url: 'https://img.example.com/?apiKey=sk-1' })
    const w = mountView()
    await flushPromises()

    expect(w.find('iframe').exists()).toBe(false)
    expect(w.find('[data-testid="launch-embed-shell"]').exists()).toBe(true)
    expect(getFrame('img', 7)).toMatchObject({ url: 'https://img.example.com/?apiKey=sk-1', hideOpenButton: false })
    w.unmount()
  })

  it('切走再回来不重新解析、不重建页面', async () => {
    launchMenu.mockResolvedValue({ kind: 'ok', url: 'https://img.example.com/?apiKey=sk-1' })
    const first = mountView()
    await flushPromises()
    first.unmount()
    const before = getFrame('img', 7)

    const w = mountView()
    await flushPromises()
    expect(launchMenu).toHaveBeenCalledTimes(1)
    expect(getFrame('img', 7)).toBe(before)
    expect(w.find('[data-testid="launch-embed-shell"]').exists()).toBe(true)
    w.unmount()
  })

  it('换了用户要重新解析', async () => {
    launchMenu.mockResolvedValue({ kind: 'ok', url: 'https://img.example.com/?apiKey=sk-1' })
    const first = mountView()
    await flushPromises()
    first.unmount()
    authStore.user = { id: 8 }
    launchMenu.mockResolvedValue({ kind: 'ok', url: 'https://img.example.com/?apiKey=sk-2' })
    const second = mountView()
    await flushPromises()
    second.unmount()
    expect(launchMenu).toHaveBeenCalledTimes(2)
    expect(getFrame('img', 8)?.url).toContain('sk-2')
  })

  it('同一实例切换菜单时解析新菜单，并带上隐藏按钮配置', async () => {
    launchMenu.mockImplementation(async (id: string) => ({ kind: 'ok', url: `https://img.example.com/?m=${id}` }))
    const w = mountView()
    await flushPromises()
    route.params.id = 'img2'
    await flushPromises()
    expect(launchMenu).toHaveBeenLastCalledWith('img2')
    expect(getFrame('img2', 7)?.hideOpenButton).toBe(true)
    w.unmount()
    expect(embedHostState.activeId).toBeNull()
  })

  it('被拒绝时显示原因，不创建页面', async () => {
    launchMenu.mockResolvedValue({
      kind: 'blocked',
      reason: { code: 'no_balance', message: '余额不足，请先充值后再使用', action: { label: '去充值', path: '/purchase' } },
    })
    const w = mountView()
    await flushPromises()
    expect(w.text()).toContain('余额不足')
    expect(w.text()).toContain('去充值')
    expect(embedHostState.frames).toHaveLength(0)
    w.unmount()
  })

  it('new_tab 模式直接跳转，不进常驻层', async () => {
    route.params.id = 'tab'
    const replace = vi.fn()
    const original = window.location
    Object.defineProperty(window, 'location', { configurable: true, value: { ...original, replace } })
    launchMenu.mockResolvedValue({ kind: 'ok', url: 'https://img.example.com/?apiKey=sk-1' })
    const w = mountView()
    await flushPromises()
    expect(replace).toHaveBeenCalledWith('https://img.example.com/?apiKey=sk-1')
    expect(embedHostState.frames).toHaveLength(0)
    w.unmount()
    Object.defineProperty(window, 'location', { configurable: true, value: original })
  })
})
