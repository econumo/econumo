import { describe, expect, it } from 'vitest'
import { anchoredTop, sheetVisibleHeight } from './useAnchoredSheet'

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
