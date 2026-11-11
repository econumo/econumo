import { describe, expect, it } from 'vitest'
import { anchoredTop } from './useAnchoredSheet'

describe('anchoredTop', () => {
  // a long list on an 812px screen: 80% tall, handle + title + search 146px (measured)
  const long = { top: 162, natural: 1800, chrome: 146 }

  it('keeps the top where it opened while the keyboard leaves two rows', () => {
    expect(anchoredTop(long, 812)).toBe(162)
    // the number keyboard, then the taller text keyboard (measured)
    expect(anchoredTop(long, 442)).toBe(162)
    expect(anchoredTop(long, 416)).toBe(162)
  })

  it('moves the top up only as far as the keyboard needs', () => {
    expect(anchoredTop(long, 380)).toBe(134)
    expect(anchoredTop(long, 200)).toBe(24)
  })

  it('lifts a short list just above the keyboard', () => {
    const short = { top: 600, natural: 212, chrome: 146 }
    expect(anchoredTop(short, 812)).toBe(600)
    expect(anchoredTop(short, 416)).toBe(204)
  })
})
