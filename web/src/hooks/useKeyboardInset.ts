import { useEffect, useState } from 'react'

export interface KeyboardInset {
  /** px between the layout viewport's bottom and the visible area's bottom (the on-screen keyboard) */
  bottom: number
  /** px height of the visible area */
  height: number
  /** px the visible area's top sits below the layout viewport's top (iOS pans it) */
  top: number
}

const NONE: KeyboardInset = { bottom: 0, height: 0, top: 0 }

// a keyboard is never shorter than this; smaller drops of the visible area
// against the tallest one seen are browser toolbars, not a keyboard
const MIN_KEYBOARD_PX = 150
// iOS sometimes reports the keyboard before it has finished panning or
// resizing, and then fires nothing more: measure again once it has settled
const SETTLE_MS = [100, 300, 600]

const NO_KEYBOARD_INPUTS = new Set(['button', 'checkbox', 'radio', 'range', 'color', 'file', 'submit', 'reset', 'image', 'hidden'])

function keyboardFieldFocused(): boolean {
  const el = document.activeElement
  if (el instanceof HTMLTextAreaElement) {
    return el.inputMode !== 'none'
  }
  if (el instanceof HTMLInputElement) {
    return el.inputMode !== 'none' && !NO_KEYBOARD_INPUTS.has(el.type)
  }
  return el instanceof HTMLElement && el.isContentEditable
}

// the full layout viewport height per window width. The iOS home-screen app
// sometimes shrinks the layout viewport with the keyboard too, so the visible
// area then matches innerHeight and only this baseline still shows the
// keyboard. It only keeps a larger height while a field that raises a
// keyboard has focus; otherwise the window really is that size (split screen,
// a resized desktop window) and the current height is the baseline
const tallest = new Map<number, number>()

function tallestHeight(): number {
  const width = window.innerWidth
  const seen = tallest.get(width)
  const height = seen !== undefined && keyboardFieldFocused() ? Math.max(seen, window.innerHeight) : window.innerHeight
  tallest.set(width, height)
  return height
}

// recorded from app start, not only while a sheet is open: a picker opened
// from a page field whose keyboard already shrank the page would otherwise
// take the shrunk height for the full one
if (typeof window !== 'undefined') {
  tallestHeight()
  window.addEventListener('resize', tallestHeight)
}

/** test-only: forget the tallest viewport seen */
export function resetKeyboardBaseline() {
  tallest.clear()
}

function measure(): KeyboardInset {
  const vv = window.visualViewport
  if (!vv) {
    return NONE
  }
  // a fixed element is placed against the layout viewport, which the keyboard
  // usually does not shrink; iOS may also pan it (offsetTop), so the visible bottom
  // edge is offsetTop + height. A fully panned viewport needs no lift but is
  // still short, so the height cap applies whenever the keyboard is up
  const height = Math.round(vv.height)
  if (height >= window.innerHeight && height > tallestHeight() - MIN_KEYBOARD_PX) {
    return NONE
  }
  // the visible area never reaches below the layout viewport: with the layout
  // viewport shrunk to it, iOS still reports an offsetTop (396 of a 416px
  // viewport, measured) for a pan it has not done
  const top = Math.min(Math.round(vv.offsetTop), Math.max(0, window.innerHeight - height))
  return { bottom: Math.max(0, window.innerHeight - top - height), height, top }
}

// Tracks the visual viewport continuously instead of guessing an open/closed
// keyboard state: iOS resizes the viewport for the suggestion bar, a keyboard
// switch or the emoji keyboard too, and a toggled flag drifts out of sync
export function useKeyboardInset(enabled: boolean): KeyboardInset {
  const [inset, setInset] = useState<KeyboardInset>(NONE)
  useEffect(() => {
    const vv = window.visualViewport
    if (!enabled || !vv) {
      setInset(NONE)
      return
    }
    const update = () => {
      const next = measure()
      setInset((prev) => (prev.bottom === next.bottom && prev.height === next.height && prev.top === next.top ? prev : next))
    }
    const timers: ReturnType<typeof setTimeout>[] = []
    const settle = () => {
      update()
      timers.forEach(clearTimeout)
      timers.length = 0
      SETTLE_MS.forEach((ms) => timers.push(setTimeout(update, ms)))
    }
    settle()
    vv.addEventListener('resize', settle)
    vv.addEventListener('scroll', update)
    window.addEventListener('resize', settle)
    // a field tapped in an open sheet raises the keyboard, also a tap on the
    // field that already has focus after the keyboard was hidden
    document.addEventListener('focusin', settle)
    document.addEventListener('touchend', settle)
    return () => {
      timers.forEach(clearTimeout)
      vv.removeEventListener('resize', settle)
      vv.removeEventListener('scroll', update)
      window.removeEventListener('resize', settle)
      document.removeEventListener('focusin', settle)
      document.removeEventListener('touchend', settle)
    }
  }, [enabled])
  return inset
}
