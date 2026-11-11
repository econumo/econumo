import { renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useSheetGlide } from './useSheetGlide'

function sheet(top: () => number) {
  const el = document.createElement('div')
  el.getBoundingClientRect = () => ({ top: top() }) as DOMRect
  const animate = vi.fn()
  el.animate = animate as unknown as HTMLElement['animate']
  el.getAnimations = () => []
  return { el, animate }
}

describe('useSheetGlide', () => {
  afterEach(() => vi.restoreAllMocks())

  it('slides the sheet from where it was when the keyboard moves it', () => {
    let top = 24
    const { el, animate } = sheet(() => top)
    const ref = { current: el }
    const { rerender } = renderHook(({ placement }) => useSheetGlide(ref, true, placement), { initialProps: { placement: 'up' } })
    // the keyboard went away: iOS shows the sheet at its full size at once
    top = 113
    rerender({ placement: 'down' })
    expect(animate).toHaveBeenCalledTimes(1)
    expect(animate.mock.calls[0][0]).toEqual([{ translate: '0 -89px' }, { translate: '0 0' }])
  })

  it('only records the place when the list changes size', () => {
    let top = 24
    const { el, animate } = sheet(() => top)
    const ref = { current: el }
    const { rerender } = renderHook(({ placement }) => useSheetGlide(ref, true, placement), { initialProps: { placement: 'up' } })
    // typing filtered the list down
    top = 300
    rerender({ placement: 'up' })
    expect(animate).not.toHaveBeenCalled()
    // then the keyboard went away: the glide starts from 300, not the stale 24
    top = 600
    rerender({ placement: 'down' })
    expect(animate.mock.calls[0][0]).toEqual([{ translate: '0 -300px' }, { translate: '0 0' }])
  })

  it('continues from where a glide cut short had got to', () => {
    let top = 24
    const { el, animate } = sheet(() => top)
    const ref = { current: el }
    const { rerender } = renderHook(({ placement }) => useSheetGlide(ref, true, placement), { initialProps: { placement: 'up' } })
    top = 113
    rerender({ placement: 'down' })
    // halfway through: the sheet still shows 40px above its rest place (jsdom
    // computes no translate, so the glide's value is stubbed)
    vi.spyOn(window, 'getComputedStyle').mockReturnValue({ translate: '0px -40px', transform: 'none' } as CSSStyleDeclaration)
    top = 73
    rerender({ placement: 'down' })
    expect(animate).toHaveBeenCalledTimes(1)
    // the keyboard comes back: it rests at 24 now, starting from 73 on screen
    top = 24 - 40
    rerender({ placement: 'up' })
    expect(animate.mock.calls[1][0]).toEqual([{ translate: '0 49px' }, { translate: '0 0' }])
  })

  it('still glides after a drag that left an identity transform behind', () => {
    let top = 24
    const { el, animate } = sheet(() => top)
    // as a browser computes vaul's leftover translate3d(0, 0, 0)
    el.style.transform = 'matrix(1, 0, 0, 1, 0, 0)'
    const ref = { current: el }
    const { rerender } = renderHook(({ placement }) => useSheetGlide(ref, true, placement), { initialProps: { placement: 'up' } })
    top = 113
    rerender({ placement: 'down' })
    expect(animate).toHaveBeenCalledTimes(1)
  })

  it('leaves the sheet to vaul while it slides in or out', () => {
    let top = 24
    const { el, animate } = sheet(() => top)
    el.style.transform = 'translateY(100px)'
    const ref = { current: el }
    const { rerender } = renderHook(({ placement }) => useSheetGlide(ref, true, placement), { initialProps: { placement: 'up' } })
    top = 113
    rerender({ placement: 'down' })
    expect(animate).not.toHaveBeenCalled()
  })
})
