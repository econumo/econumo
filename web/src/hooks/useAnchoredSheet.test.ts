import { describe, expect, it } from 'vitest'
import { anchoredTop } from './useAnchoredSheet'

describe('anchoredTop', () => {
  // a long list on an 812px screen fills it up to the status bar (54px + 8);
  // handle, title and search take 146px (measured)
  const long = { top: 62, natural: 1800, chrome: 146, minTop: 62, layout: 812 }

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
    const short = { top: 600, natural: 212, chrome: 146, minTop: 62, layout: 812 }
    expect(anchoredTop(short, 812)).toBe(600)
    expect(anchoredTop(short, 416)).toBe(204)
  })

  it('keeps a mid-size list\'s top until the keyboard leaves under two rows', () => {
    const mid = { top: 300, natural: 512, chrome: 146, minTop: 62, layout: 812 }
    expect(anchoredTop(mid, 442)).toBe(196)
    expect(anchoredTop(mid, 812)).toBe(300)
  })
})
