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

// the tallest layout viewport seen at the current width (a rotation resets
// it). The iOS home-screen app sometimes shrinks the layout viewport with the
// keyboard too, so the visible area then matches innerHeight and only this
// baseline still shows the keyboard
let tallest = { width: 0, height: 0 }

function tallestHeight(): number {
  if (tallest.width !== window.innerWidth) {
    tallest = { width: window.innerWidth, height: 0 }
  }
  tallest.height = Math.max(tallest.height, window.innerHeight)
  return tallest.height
}

/** test-only: forget the tallest viewport seen */
export function resetKeyboardBaseline() {
  tallest = { width: 0, height: 0 }
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
  // with the layout viewport shrunk to the visible area there is nothing to
  // pan, yet iOS still reports an offsetTop (396 of a 416px viewport, measured)
  const top = height >= window.innerHeight ? 0 : Math.round(vv.offsetTop)
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
    // a field tapped in an open sheet raises the keyboard
    document.addEventListener('focusin', settle)
    return () => {
      timers.forEach(clearTimeout)
      vv.removeEventListener('resize', settle)
      vv.removeEventListener('scroll', update)
      window.removeEventListener('resize', settle)
      document.removeEventListener('focusin', settle)
    }
  }, [enabled])
  return inset
}
