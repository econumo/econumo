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

function measure(): KeyboardInset {
  const vv = window.visualViewport
  if (!vv) {
    return NONE
  }
  // a fixed element is placed against the layout viewport, which the keyboard
  // does not shrink; iOS may also pan it (offsetTop), so the visible bottom
  // edge is offsetTop + height. A fully panned viewport needs no lift but is
  // still short, so the height cap applies whenever the keyboard is up
  const height = Math.round(vv.height)
  if (height >= window.innerHeight) {
    return NONE
  }
  const top = Math.round(vv.offsetTop)
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
    update()
    vv.addEventListener('resize', update)
    vv.addEventListener('scroll', update)
    return () => {
      vv.removeEventListener('resize', update)
      vv.removeEventListener('scroll', update)
    }
  }, [enabled])
  return inset
}
