import { useLayoutEffect, useRef, useState } from 'react'
import type { CSSProperties, RefObject } from 'react'
import type { KeyboardInset } from '@/hooks/useKeyboardInset'

// px kept free above the sheet
export const SHEET_TOP_GAP = 24
// px between the status bar and a sheet that fills the screen
const STATUS_BAR_GAP = 8
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
  /** px the top never goes above: clear of the status bar */
  minTop: number
}

let safeTop: { width: number; px: number } | null = null

// the status bar's height on an edge-to-edge screen (viewport-fit=cover)
function safeAreaTop(): number {
  if (safeTop?.width !== window.innerWidth) {
    const probe = document.createElement('div')
    probe.style.cssText = 'position:fixed;top:0;visibility:hidden;height:env(safe-area-inset-top, 0px)'
    document.body.append(probe)
    safeTop = { width: window.innerWidth, px: probe.offsetHeight }
    probe.remove()
  }
  return safeTop.px
}

/** where the sheet's top sits on screen for the visible area left by the keyboard */
export function anchoredTop(anchor: SheetAnchor, visibleHeight: number): number {
  const room = Math.min(anchor.chrome + MIN_LIST, anchor.natural)
  return Math.max(anchor.minTop, Math.min(anchor.top, visibleHeight - room))
}

/**
 * A sheet whose top stays where it opened: the keyboard coming and going only
 * moves its bottom edge, and the list in between gets shorter or taller. The
 * top is set once from the content: as tall as the content, and a list
 * longer than the screen fills it up to the status bar, so the keyboard
 * leaves it room for several rows. It moves up when the content grows (data
 * arriving), never down when it shrinks (a filter), and moves up only as far
 * as needed when the keyboard would leave less than about two rows of list.
 * A closing sheet keeps its place while it slides out.
 */
export interface AnchoredSheet {
  style: CSSProperties
  /** px from the top of the visible area to the sheet's top */
  top: number
}

export function useAnchoredSheet(
  ref: RefObject<HTMLElement | null>,
  body: HTMLElement | null,
  enabled: boolean,
  open: boolean,
  keyboard: KeyboardInset,
): AnchoredSheet | undefined {
  const [anchor, setAnchor] = useState<SheetAnchor | null>(null)
  const measured = useRef(false)
  // after every render, since the content can grow without the keyboard
  // changing; it only ever sets the anchor higher, so it settles
  // oxlint-disable-next-line react-hooks/exhaustive-deps
  useLayoutEffect(() => {
    if (!enabled) {
      measured.current = false
      if (anchor !== null) {
        setAnchor(null)
      }
      return
    }
    // a closing sheet keeps its place while it slides out; the next opening
    // measures afresh
    if (!open) {
      measured.current = false
      return
    }
    const el = ref.current
    if (!el || !body) {
      return
    }
    // the content's own height, however tall the sheet is drawn now
    const chrome = el.offsetHeight - body.clientHeight
    const natural = chrome + body.scrollHeight
    const layout = document.documentElement.clientHeight || window.innerHeight
    const minTop = Math.max(SHEET_TOP_GAP, safeAreaTop() + STATUS_BAR_GAP)
    const top = Math.max(minTop, layout - natural)
    if (!measured.current || anchor === null || top < anchor.top - 1) {
      measured.current = true
      setAnchor({ top, natural, chrome, minTop })
    }
  })
  if (!enabled || anchor === null) {
    return undefined
  }
  const layout = document.documentElement.clientHeight || window.innerHeight
  const visibleHeight = keyboard.height > 0 ? keyboard.height : layout
  const top = anchoredTop(anchor, visibleHeight)
  return {
    top,
    // placed in the layout viewport, which iOS pans by keyboard.top;
    // marginTop: the drawer's mt-24 only kept a bottom-anchored sheet off the top
    style: { top: keyboard.top + top, bottom: 'auto', marginTop: 0, height: visibleHeight - top, maxHeight: 'none' },
  }
}
