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
 * Slides the sheet from its previous place to its new one when its top moves
 * (a short list rising above the keyboard), instead of jumping. `top` is the
 * place the sheet was given (px from the top of the visible area), not a
 * measurement: iOS reports the sheet's position inconsistently while it pans
 * for the keyboard (measured: glides started 47-72px off), and a long list's
 * top never moves, so it never glides. It animates `translate`, which
 * composes with vaul's own `transform`.
 */
export function useSheetGlide(ref: RefObject<HTMLElement | null>, active: boolean, top: number | undefined) {
  const last = useRef<number | undefined>(undefined)
  useLayoutEffect(() => {
    const el = ref.current
    const prev = last.current
    last.current = active ? top : undefined
    if (!active || !el || prev === undefined || top === undefined || Math.abs(prev - top) < 2) {
      return
    }
    // a glide cut short continues from where it had got to
    const from = prev - top + glideOffset(el)
    el.getAnimations?.().forEach((a) => a.id === GLIDE_ID && a.cancel())
    if (typeof el.animate !== 'function' || vaulMoving(el) || window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) {
      return
    }
    el.animate([{ translate: `0 ${from}px` }, { translate: '0 0' }], { duration: GLIDE_MS, easing: GLIDE_EASING, id: GLIDE_ID })
  }, [ref, active, top])
}
