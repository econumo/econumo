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
