import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
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

  it('sees the keyboard when iOS reports innerHeight as the visible height', () => {
    Object.defineProperty(document.documentElement, 'clientHeight', { configurable: true, value: 800 })
    try {
      const { result } = renderHook(() => useKeyboardInset(true))
      act(() => {
        Object.defineProperty(window, 'innerHeight', { configurable: true, value: 450 })
      })
      resize(450, 350)
      expect(result.current).toEqual({ bottom: 0, height: 450, top: 350 })
      // the keyboard going away while the search loses focus: still the keyboard
      // until the visible area grows back
      act(() => {
        ;(document.activeElement as HTMLElement | null)?.blur()
      })
      resize(450, 350)
      expect(result.current).toEqual({ bottom: 0, height: 450, top: 350 })
      act(() => {
        Object.defineProperty(window, 'innerHeight', { configurable: true, value: 800 })
      })
      resize(800)
      expect(result.current).toEqual({ bottom: 0, height: 0, top: 0 })
    } finally {
      Object.defineProperty(document.documentElement, 'clientHeight', { configurable: true, value: 0 })
    }
  })

  it('takes a window that is really that short for no keyboard', () => {
    const { result } = renderHook(() => useKeyboardInset(true))
    act(() => {
      Object.defineProperty(window, 'innerHeight', { configurable: true, value: 450 })
    })
    resize(450)
    expect(result.current).toEqual({ bottom: 0, height: 0, top: 0 })
  })

  it('never places the visible area below the layout viewport', () => {
    const { result } = renderHook(() => useKeyboardInset(true))
    resize(450, 400)
    expect(result.current).toEqual({ bottom: 0, height: 450, top: 350 })
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

  it('applies a viewport change within the event, not a render later', () => {
    const { result } = renderHook(() => useKeyboardInset(true))
    // no act(): the new inset must already be rendered when dispatch returns
    const env = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    const actEnv = env.IS_REACT_ACT_ENVIRONMENT
    env.IS_REACT_ACT_ENVIRONMENT = false
    try {
      vv.height = 450
      vv.dispatchEvent(new Event('resize'))
    } finally {
      env.IS_REACT_ACT_ENVIRONMENT = actEnv
    }
    expect(result.current).toEqual({ bottom: 350, height: 450, top: 0 })
  })

  it('stays idle when disabled', () => {
    const { result } = renderHook(() => useKeyboardInset(false))
    resize(450)
    expect(result.current).toEqual({ bottom: 0, height: 0, top: 0 })
  })
})
