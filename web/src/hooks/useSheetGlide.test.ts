import { renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useSheetGlide } from './useSheetGlide'

function sheet() {
  const el = document.createElement('div')
  const animate = vi.fn()
  el.animate = animate as unknown as HTMLElement['animate']
  el.getAnimations = () => []
  return { el, animate }
}

describe('useSheetGlide', () => {
  afterEach(() => vi.restoreAllMocks())

  it('slides a sheet whose top moves from its old place to the new one', () => {
    const { el, animate } = sheet()
    const ref = { current: el }
    const { rerender } = renderHook(({ top }) => useSheetGlide(ref, true, top), { initialProps: { top: 600 } })
    // a short list rises above the keyboard
    rerender({ top: 204 })
    expect(animate).toHaveBeenCalledTimes(1)
    expect(animate.mock.calls[0][0]).toEqual([{ translate: '0 396px' }, { translate: '0 0' }])
  })

  it('never glides a sheet whose top stays put', () => {
    const { el, animate } = sheet()
    const ref = { current: el }
    const { rerender } = renderHook(({ top }) => useSheetGlide(ref, true, top), { initialProps: { top: 24 } })
    rerender({ top: 24 })
    rerender({ top: 25 })
    expect(animate).not.toHaveBeenCalled()
  })

  it('continues from where a glide cut short had got to', () => {
    const { el, animate } = sheet()
    const ref = { current: el }
    const { rerender } = renderHook(({ top }) => useSheetGlide(ref, true, top), { initialProps: { top: 600 } })
    rerender({ top: 204 })
    // halfway back: the sheet still shows 100px below its new place (jsdom
    // computes no translate, so the glide's value is stubbed)
    vi.spyOn(window, 'getComputedStyle').mockReturnValue({ translate: '0px 100px', transform: 'none' } as CSSStyleDeclaration)
    rerender({ top: 600 })
    expect(animate.mock.calls[1][0]).toEqual([{ translate: '0 -296px' }, { translate: '0 0' }])
  })

  it('starts afresh each time the sheet opens', () => {
    const { el, animate } = sheet()
    const ref = { current: el }
    const { rerender } = renderHook(({ active, top }) => useSheetGlide(ref, active, top), { initialProps: { active: true, top: 600 } })
    rerender({ active: false, top: 600 })
    rerender({ active: true, top: 204 })
    expect(animate).not.toHaveBeenCalled()
  })

  it('still glides after a drag that left an identity transform behind', () => {
    const { el, animate } = sheet()
    // as a browser computes vaul's leftover translate3d(0, 0, 0)
    el.style.transform = 'matrix(1, 0, 0, 1, 0, 0)'
    const ref = { current: el }
    const { rerender } = renderHook(({ top }) => useSheetGlide(ref, true, top), { initialProps: { top: 600 } })
    rerender({ top: 204 })
    expect(animate).toHaveBeenCalledTimes(1)
  })

  it('leaves the sheet to vaul while it slides in or out', () => {
    const { el, animate } = sheet()
    el.style.transform = 'translateY(100px)'
    const ref = { current: el }
    const { rerender } = renderHook(({ top }) => useSheetGlide(ref, true, top), { initialProps: { top: 600 } })
    rerender({ top: 204 })
    expect(animate).not.toHaveBeenCalled()
  })
})
