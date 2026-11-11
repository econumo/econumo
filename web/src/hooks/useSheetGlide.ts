import { useLayoutEffect, useRef } from 'react'
import type { RefObject } from 'react'

const GLIDE_MS = 200
// vaul's own open/close curve, so the glide reads as the same sheet moving
const GLIDE_EASING = 'cubic-bezier(0.32, 0.72, 0, 1)'
const GLIDE_ID = 'sheet-glide'

/**
 * iOS moves the visible area in one step on the frame its keyboard starts to
 * rise or fall (measured: 812px <-> 416px panned by 396px), so a sheet that
 * sits on the keyboard jumps. Each time the keyboard changes the sheet's
 * place, this starts it where it was on screen and slides it to its new place.
 * It animates `translate`, which composes with vaul's own `transform`.
 *
 * `placement` changes exactly when the keyboard moves the sheet; every other
 * render (a filtered list growing or shrinking) only records where it is.
 */
export function useSheetGlide(ref: RefObject<HTMLElement | null>, active: boolean, placement: string) {
  const last = useRef<{ top: number; placement: string } | null>(null)
  useLayoutEffect(() => {
    const el = ref.current
    if (!active || !el) {
      last.current = null
      return
    }
    const prev = last.current
    const moved = prev !== null && prev.placement !== placement
    if (moved) {
      // measured from where it is now, without an earlier glide still running
      el.getAnimations?.().forEach((a) => a.id === GLIDE_ID && a.cancel())
    }
    const top = el.getBoundingClientRect().top
    last.current = { top, placement }
    if (!moved || Math.abs(prev.top - top) < 2 || typeof el.animate !== 'function') {
      return
    }
    // vaul is sliding the sheet in or out, or the user is dragging it
    if (getComputedStyle(el).transform !== 'none') {
      return
    }
    if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) {
      return
    }
    el.animate([{ translate: `0 ${prev.top - top}px` }, { translate: '0 0' }], { duration: GLIDE_MS, easing: GLIDE_EASING, id: GLIDE_ID })
  })
}
