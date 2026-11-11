import { useLayoutEffect, useRef } from 'react'
import type { RefObject } from 'react'

const GLIDE_MS = 200
// vaul's own open/close curve, so the glide reads as the same sheet moving
const GLIDE_EASING = 'cubic-bezier(0.32, 0.72, 0, 1)'
const GLIDE_ID = 'sheet-glide'

// px the running glide still offsets the sheet (its animated `translate`)
function glideOffset(el: HTMLElement): number {
  // a running animation's value is only in the computed style
  const value = getComputedStyle(el).translate
  if (!value || value === 'none') {
    return 0
  }
  const y = parseFloat(value.split(' ')[1] ?? '0')
  return Number.isFinite(y) ? y : 0
}

// vaul slides the sheet in and out, and follows a drag, with `transform`; a
// drag released without closing leaves an identity transform behind
function vaulMoving(el: HTMLElement): boolean {
  const value = getComputedStyle(el).transform
  if (!value || value === 'none') {
    return false
  }
  const parts = value.match(/matrix(?:3d)?\(([^)]*)\)/)?.[1].split(',').map(Number)
  if (!parts) {
    return true
  }
  const ty = parts.length === 16 ? parts[13] : parts[5]
  return Math.abs(ty) > 0.5
}

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
  // where the sheet rests, without a glide's offset
  const last = useRef<{ top: number; placement: string } | null>(null)
  useLayoutEffect(() => {
    const el = ref.current
    if (!active || !el) {
      last.current = null
      return
    }
    // read before cancelling: where a glide cut short has got to
    const offset = glideOffset(el)
    const top = el.getBoundingClientRect().top - offset
    const prev = last.current
    last.current = { top, placement }
    if (prev === null || prev.placement === placement) {
      return
    }
    el.getAnimations?.().forEach((a) => a.id === GLIDE_ID && a.cancel())
    // on screen it was at its rest place plus the glide's offset
    const from = prev.top + offset - top
    if (Math.abs(from) < 2 || typeof el.animate !== 'function' || vaulMoving(el)) {
      return
    }
    if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) {
      return
    }
    el.animate([{ translate: `0 ${from}px` }, { translate: '0 0' }], { duration: GLIDE_MS, easing: GLIDE_EASING, id: GLIDE_ID })
  })
}
