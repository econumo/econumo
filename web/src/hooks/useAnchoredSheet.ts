import { useLayoutEffect, useState } from 'react'
import type { CSSProperties, RefObject } from 'react'
import type { KeyboardInset } from '@/hooks/useKeyboardInset'

// px kept free above the sheet
export const SHEET_TOP_GAP = 24
// a sheet without the keyboard is at most this share of the screen
const MAX_SHARE = 0.8
// px of list the keyboard must leave (about two rows) before the sheet's top
// moves up
const MIN_LIST = 100

export interface SheetAnchor {
  /** px from the top of the visible area, chosen as the sheet opens */
  top: number
  /** px the sheet's content takes */
  natural: number
  /** px of it that is not the scrolling list: handle, title, search */
  chrome: number
}

/** where the sheet's top sits on screen for the visible area left by the keyboard */
export function anchoredTop(anchor: SheetAnchor, visibleHeight: number): number {
  const room = Math.min(anchor.chrome + MIN_LIST, anchor.natural)
  return Math.max(SHEET_TOP_GAP, Math.min(anchor.top, visibleHeight - room))
}

/**
 * A sheet whose top stays where it opened: the keyboard coming and going only
 * moves its bottom edge, and the list in between gets shorter or taller. The
 * top is set once from the content (as tall as the content, up to 80% of the
 * screen), moves up when the content grows (data arriving), never down when
 * it shrinks (a filter), and moves up only as far as needed when the keyboard
 * would leave less than about two rows of list.
 */
export function useAnchoredSheet(
  ref: RefObject<HTMLElement | null>,
  bodyRef: RefObject<HTMLElement | null>,
  enabled: boolean,
  keyboard: KeyboardInset,
): CSSProperties | undefined {
  const [anchor, setAnchor] = useState<SheetAnchor | null>(null)
  // after every render, since the content can grow without the keyboard
  // changing; it only ever sets the anchor higher, so it settles
  // oxlint-disable-next-line react-hooks/exhaustive-deps
  useLayoutEffect(() => {
    if (!enabled) {
      if (anchor !== null) {
        setAnchor(null)
      }
      return
    }
    const el = ref.current
    const body = bodyRef.current
    if (!el || !body) {
      return
    }
    // the content's own height, however tall the sheet is drawn now
    const chrome = el.offsetHeight - body.clientHeight
    const natural = chrome + body.scrollHeight
    const layout = document.documentElement.clientHeight || window.innerHeight
    const top = Math.max(SHEET_TOP_GAP, layout - Math.min(natural, Math.round(layout * MAX_SHARE)))
    if (anchor === null || top < anchor.top - 1) {
      setAnchor({ top, natural, chrome })
    }
  })
  if (!enabled || anchor === null) {
    return undefined
  }
  const layout = document.documentElement.clientHeight || window.innerHeight
  const visibleHeight = keyboard.height > 0 ? keyboard.height : layout
  const top = anchoredTop(anchor, visibleHeight)
  // placed in the layout viewport, which iOS pans by keyboard.top
  // marginTop: the drawer's mt-24 only kept a bottom-anchored sheet off the top
  return { top: keyboard.top + top, bottom: 'auto', marginTop: 0, height: visibleHeight - top, maxHeight: 'none' }
}
