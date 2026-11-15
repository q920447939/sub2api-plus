import { beforeEach, describe, expect, it } from 'vitest'
import {
  MAX_LIVE_FRAMES,
  activate,
  clearFrames,
  deactivate,
  embedHostState,
  getFrame,
  markLoaded,
  openFrame,
  touchFrame,
} from '../embedHost'

const rect = { top: 80, left: 260, width: 900, height: 600 }
const frame = (menuId: string, url = `https://img.example.com/?apiKey=${menuId}`, ownerId: number | null = 1) => ({
  menuId,
  url,
  ownerId,
  hideOpenButton: false,
})

describe('embedHost', () => {
  beforeEach(() => clearFrames())

  it('只在同一用户下复用已打开的页面', () => {
    openFrame(frame('a'))
    expect(getFrame('a', 1)?.url).toContain('apiKey=a')
    expect(getFrame('a', 2)).toBeUndefined()
  })

  it('重复打开同一地址保留加载状态，换了地址则重新加载', () => {
    openFrame(frame('a'))
    markLoaded('a')
    openFrame(frame('a'))
    expect(getFrame('a', 1)?.loaded).toBe(true)
    openFrame(frame('a', 'https://img.example.com/?apiKey=new'))
    expect(getFrame('a', 1)?.loaded).toBe(false)
  })

  it('超出上限时淘汰最久没用的，且不改变其余页面的顺序', () => {
    for (let i = 0; i < MAX_LIVE_FRAMES; i++) openFrame(frame(`m${i}`))
    touchFrame('m0') // m1 becomes the least recently used
    openFrame(frame('extra'))

    const ids = embedHostState.frames.map((f) => f.menuId)
    expect(ids).not.toContain('m1')
    // DOM order must stay stable: moving an iframe node reloads it
    expect(ids).toEqual(['m0', 'm2', 'extra'])
  })

  it('只有当前显示者能隐藏页面', () => {
    openFrame(frame('a'))
    openFrame(frame('b'))
    activate('a', rect)
    activate('b', rect)
    deactivate('a') // stale view leaving must not hide b
    expect(embedHostState.activeId).toBe('b')
    deactivate('b')
    expect(embedHostState.activeId).toBeNull()
    expect(embedHostState.frames).toHaveLength(2)
  })

  it('被淘汰的页面正在显示时，一并清掉显示状态', () => {
    openFrame(frame('a'))
    activate('a', rect)
    for (let i = 0; i < MAX_LIVE_FRAMES; i++) openFrame(frame(`x${i}`))
    expect(getFrame('a', 1)).toBeUndefined()
    expect(embedHostState.activeId).toBeNull()
  })
})
