import { useLayoutEffect, useState } from 'react'
import { flushSync } from 'react-dom'

export interface KeyboardInset {
  /** px between the layout viewport's bottom and the visible area's bottom (the on-screen keyboard) */
  bottom: number
  /** px height of the visible area */
  height: number
  /** px the visible area's top sits below the layout viewport's top (iOS pans it) */
  top: number
}

const NONE: KeyboardInset = { bottom: 0, height: 0, top: 0 }

// iOS sometimes reports the keyboard before it has finished panning, and then
// fires nothing more: measure again once it has settled
const SETTLE_MS = [100, 300, 600]

// The layout viewport's height. The iOS home-screen app sometimes reports
// innerHeight as the visible height while the keyboard is up (measured:
// innerHeight 416 = visualViewport.height, offsetTop 396 on an 812px screen),
// so innerHeight alone would hide the keyboard; clientHeight is not affected

function layoutHeight(): number {
  return Math.max(document.documentElement.clientHeight, window.innerHeight)
}

function measure(): KeyboardInset {
  const vv = window.visualViewport
  if (!vv) {
    return NONE
  }
  // a fixed element is placed against the layout viewport, which the keyboard
  // does not shrink; iOS may also pan it (offsetTop), so the visible bottom
  // edge is offsetTop + height. A fully panned viewport needs no lift but is
  // still short, so the height cap applies whenever the keyboard is up
  const layout = layoutHeight()
  const height = Math.round(vv.height)
  if (height >= layout) {
    return NONE
  }
  const top = Math.min(Math.max(0, Math.round(vv.offsetTop)), layout - height)
  return { bottom: layout - top - height, height, top }
}

// Tracks the visual viewport continuously instead of guessing an open/closed
// keyboard state: iOS resizes the viewport for the suggestion bar, a keyboard
// switch or the emoji keyboard too, and a toggled flag drifts out of sync
export function useKeyboardInset(enabled: boolean): KeyboardInset {
  const [inset, setInset] = useState<KeyboardInset>(NONE)
  // measured before the first paint: a sheet opened over a page whose keyboard
  // is already up must not show one frame sized for no keyboard
  useLayoutEffect(() => {
    const vv = window.visualViewport
    if (!enabled || !vv) {
      setInset(NONE)
      return
    }
    // rendered within the event: iOS moves the visible area in one step and
    // paints it in the same frame, so a sheet updated a render later shows
    // one frame in the wrong place (measured: dropped 396px, or off the top)
    const apply = () => {
      const next = measure()
      setInset((prev) => (prev.bottom === next.bottom && prev.height === next.height && prev.top === next.top ? prev : next))
    }
    const update = () => flushSync(apply)
    const timers: ReturnType<typeof setTimeout>[] = []
    const later = () => {
      timers.forEach(clearTimeout)
      timers.length = 0
      SETTLE_MS.forEach((ms) => timers.push(setTimeout(update, ms)))
    }
    const settle = () => {
      update()
      later()
    }
    // in a layout effect React already renders the change before the paint
    apply()
    later()
    vv.addEventListener('resize', settle)
    vv.addEventListener('scroll', update)
    window.addEventListener('resize', settle)
    // a field tapped in an open sheet raises the keyboard, also a tap on the
    // field that already has focus after the keyboard was hidden. Only timers:
    // focus may move inside a React commit, where flushSync is not allowed
    document.addEventListener('focusin', later)
    document.addEventListener('touchend', later)
    return () => {
      timers.forEach(clearTimeout)
      vv.removeEventListener('resize', settle)
      vv.removeEventListener('scroll', update)
      window.removeEventListener('resize', settle)
      document.removeEventListener('focusin', later)
      document.removeEventListener('touchend', later)
    }
  }, [enabled])
  return inset
}
