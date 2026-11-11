import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetKeyboardBaseline, useKeyboardInset } from './useKeyboardInset'

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
    resetKeyboardBaseline()
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
    expect(result.current).toEqual({ bottom: 0, height: 0, top: 0 })
  })

  it('follows every viewport change, not an open/closed toggle', () => {
    const { result } = renderHook(() => useKeyboardInset(true))
    resize(450)
    expect(result.current).toEqual({ bottom: 350, height: 450, top: 0 })
    // the suggestion bar appearing: a 44px change must not flip any state
    resize(406)
    expect(result.current).toEqual({ bottom: 394, height: 406, top: 0 })
    resize(800)
    expect(result.current).toEqual({ bottom: 0, height: 0, top: 0 })
  })

  it('accounts for a panned layout viewport', () => {
    const { result } = renderHook(() => useKeyboardInset(true))
    resize(450, 100)
    expect(result.current).toEqual({ bottom: 250, height: 450, top: 100 })
  })

  it('keeps the height cap when the viewport is panned all the way down', () => {
    const { result } = renderHook(() => useKeyboardInset(true))
    resize(450, 350)
    expect(result.current).toEqual({ bottom: 0, height: 450, top: 350 })
  })

  it('sees a keyboard that shrank the layout viewport as well', () => {
    const { result } = renderHook(() => useKeyboardInset(true))
    act(() => {
      Object.defineProperty(window, 'innerHeight', { configurable: true, value: 450 })
    })
    // iOS reports a pan it has not done (measured: 396 of a 416px viewport)
    resize(450, 430)
    expect(result.current).toEqual({ bottom: 0, height: 450, top: 0 })
  })

  it('takes a small drop of both heights for a toolbar, not a keyboard', () => {
    const { result } = renderHook(() => useKeyboardInset(true))
    act(() => {
      Object.defineProperty(window, 'innerHeight', { configurable: true, value: 740 })
    })
    resize(740)
    expect(result.current).toEqual({ bottom: 0, height: 0, top: 0 })
  })

  it('measures again after a field takes focus, once iOS has settled', () => {
    vi.useFakeTimers()
    try {
      const { result } = renderHook(() => useKeyboardInset(true))
      act(() => {
        document.dispatchEvent(new FocusEvent('focusin'))
      })
      // iOS changed the viewport without firing an event of its own
      vv.height = 450
      act(() => {
        vi.advanceTimersByTime(700)
      })
      expect(result.current).toEqual({ bottom: 350, height: 450, top: 0 })
    } finally {
      vi.useRealTimers()
    }
  })

  it('stays idle when disabled', () => {
    const { result } = renderHook(() => useKeyboardInset(false))
    resize(450)
    expect(result.current).toEqual({ bottom: 0, height: 0, top: 0 })
  })
})
