import { renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { anchoredTop, closedTransform, sheetVisibleHeight, useAnchoredSheet } from './useAnchoredSheet'

describe('anchoredTop', () => {
  // a long list on an 812px screen fills it up to the status bar (54px + 8)
  const long = { top: 62, natural: 1800, minTop: 62, layout: 812 }

  it('keeps the top where it opened whatever the keyboard does', () => {
    expect(anchoredTop(long, 812)).toBe(62)
    // the number keyboard, then the taller text keyboard (measured)
    expect(anchoredTop(long, 442)).toBe(62)
    expect(anchoredTop(long, 416)).toBe(62)
  })

  it('never goes above the status bar', () => {
    expect(anchoredTop(long, 200)).toBe(62)
  })

  it('lifts a short list just above the keyboard', () => {
    const short = { top: 600, natural: 212, minTop: 62, layout: 812 }
    expect(anchoredTop(short, 812)).toBe(600)
    expect(anchoredTop(short, 416)).toBe(204)
  })

  it('lifts a list that fits above the keyboard just far enough to show all of it', () => {
    const few = { top: 474, natural: 400, minTop: 62, layout: 874 }
    expect(anchoredTop(few, 874)).toBe(474)
    expect(anchoredTop(few, 505)).toBe(105)
  })

  // nine income categories on an iPhone (874px, 505px above the keyboard): the
  // sheet used to keep its top and leave 100px of list, two rows of the nine
  it('lifts a list that fits the screen, but not above the keyboard, up to the status bar', () => {
    const income = { top: 271, natural: 603, minTop: 62, layout: 874 }
    expect(anchoredTop(income, 505)).toBe(62)
    // and back down to its content's height when the keyboard goes
    expect(anchoredTop(income, 874)).toBe(271)
  })
})

describe('sheetVisibleHeight', () => {
  it('is the whole screen, or what the keyboard leaves', () => {
    expect(sheetVisibleHeight(812, 0, 416, false, false)).toBe(812)
    expect(sheetVisibleHeight(812, 442, 416, false, false)).toBe(442)
  })

  it('makes room for the remembered keyboard as soon as a field is taken', () => {
    expect(sheetVisibleHeight(812, 0, 416, true, false)).toBe(416)
    // the number keyboard is still up while the taller text one comes
    expect(sheetVisibleHeight(812, 442, 416, true, false)).toBe(416)
    // a taller keyboard than remembered (emoji)
    expect(sheetVisibleHeight(812, 380, 416, true, false)).toBe(380)
  })

  it('makes no room for a keyboard never seen, or one that did not come', () => {
    expect(sheetVisibleHeight(812, 0, undefined, true, false)).toBe(812)
    expect(sheetVisibleHeight(812, 0, 416, true, true)).toBe(812)
  })
})

describe('closedTransform', () => {
  // the category picker as recorded on an iPhone: an 874px layout viewport,
  // the sheet anchored 113px down and 453px tall, so its bottom edge sits
  // 308px above the screen's. vaul's default translate of 100% (453px) ended
  // the slide with the sheet still over the lower third of the screen, where
  // it stayed until the animation's end unmounted it
  it('carries an anchored sheet right off the screen, not down by its own height', () => {
    expect(closedTransform(874, 113)).toBe('761px')
  })

  it('counts the pan when iOS has moved the visible area up for the keyboard', () => {
    expect(closedTransform(874, 113 + 396)).toBe('365px')
  })

  it('never pulls a sheet that starts below the fold back up', () => {
    expect(closedTransform(874, 900)).toBe('0px')
  })
})

describe('useAnchoredSheet closing', () => {
  // a sheet 453px tall anchored on an 874px screen, as the category picker
  // stood on the iPhone: 300px of list, the rest handle/title/search
  function sheet() {
    const el = document.createElement('div')
    const body = document.createElement('div')
    const last = document.createElement('div')
    body.append(last)
    el.append(body)
    document.body.append(el)
    Object.defineProperty(el, 'offsetHeight', { configurable: true, value: 453 })
    Object.defineProperty(body, 'clientHeight', { configurable: true, value: 300 })
    body.getBoundingClientRect = () => ({ top: 0, bottom: 300 }) as DOMRect
    last.getBoundingClientRect = () => ({ top: 0, bottom: 300 }) as DOMRect
    return { ref: { current: el }, body }
  }

  it('carries the sheet off the screen once the keyboard it stood on is gone', () => {
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 874 })
    Object.defineProperty(document.documentElement, 'clientHeight', { configurable: true, value: 874 })
    const { ref, body } = sheet()
    // the keyboard is up: 442px of visible area, iOS has not panned
    const keyboard = { bottom: 432, height: 442, top: 0 }
    const { result, rerender } = renderHook(({ open, kb }) => useAnchoredSheet(ref, body, true, open, kb), {
      initialProps: { open: true, kb: keyboard },
    })
    const top = result.current?.top ?? 0
    expect(top).toBeGreaterThan(0)
    // while open, the entrance is vaul's own, untouched
    expect(result.current?.style['--initial-transform' as keyof typeof result.current.style]).toBe('100%')

    // closing: useKeyboardInset resets to NONE, and the sheet keeps its place
    rerender({ open: false, kb: { bottom: 0, height: 0, top: 0 } })
    expect(result.current?.top).toBe(top)
    // far enough that the sheet's top edge reaches the bottom of the screen,
    // not merely its own height (453px), which left it covering the lower third
    expect(result.current?.style['--initial-transform' as keyof typeof result.current.style]).toBe(String(874 - top) + 'px')
    expect(874 - top).toBeGreaterThan(453)
  })
})
