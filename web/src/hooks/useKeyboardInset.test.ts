import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { useKeyboardInset } from './useKeyboardInset'

class FakeViewport extends EventTarget {
  height = 800
  offsetTop = 0
}

describe('useKeyboardInset', () => {
  let vv: FakeViewport
  const original = Object.getOwnPropertyDescriptor(window, 'visualViewport')

  beforeEach(() => {
    vv = new FakeViewport()
    Object.defineProperty(window, 'visualViewport', { configurable: true, value: vv })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 800 })
  })
  afterEach(() => {
    if (original) {
      Object.defineProperty(window, 'visualViewport', original)
    } else {
      delete (window as { visualViewport?: unknown }).visualViewport
    }
  })

  const resize = (height: number, offsetTop = 0) =>
    act(() => {
      vv.height = height
      vv.offsetTop = offsetTop
      vv.dispatchEvent(new Event('resize'))
    })

  it('reports nothing while the keyboard is closed', () => {
    const { result } = renderHook(() => useKeyboardInset(true))
    expect(result.current).toEqual({ bottom: 0, height: 0 })
  })

  it('follows every viewport change, not an open/closed toggle', () => {
    const { result } = renderHook(() => useKeyboardInset(true))
    resize(450)
    expect(result.current).toEqual({ bottom: 350, height: 450 })
    // the suggestion bar appearing: a 44px change must not flip any state
    resize(406)
    expect(result.current).toEqual({ bottom: 394, height: 406 })
    resize(800)
    expect(result.current).toEqual({ bottom: 0, height: 0 })
  })

  it('accounts for a panned layout viewport', () => {
    const { result } = renderHook(() => useKeyboardInset(true))
    resize(450, 100)
    expect(result.current).toEqual({ bottom: 250, height: 450 })
  })

  it('stays idle when disabled', () => {
    const { result } = renderHook(() => useKeyboardInset(false))
    resize(450)
    expect(result.current).toEqual({ bottom: 0, height: 0 })
  })
})
