import { useCallback, useEffect, useRef } from 'react'
import type { MouseEvent, PointerEvent } from 'react'

const DEFAULT_MS = 600

interface Options {
  onClick: (e: MouseEvent) => void
  onLongPress: () => void
  ms?: number
}

// A tap runs onClick, a press held for `ms` (or a Shift+click) runs onLongPress.
// onClick stays the native click so keyboard activation keeps working.
export function useLongPress({ onClick, onLongPress, ms = DEFAULT_MS }: Options) {
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const pressed = useRef(false)
  // set when a long press fired, so the click that follows its release is dropped
  const fired = useRef(false)
  const latest = useRef({ onClick, onLongPress })
  latest.current = { onClick, onLongPress }

  const stop = useCallback(() => {
    if (timer.current !== null) {
      clearTimeout(timer.current)
      timer.current = null
    }
  }, [])
  useEffect(() => stop, [stop])

  const onPointerDown = useCallback(
    (e: PointerEvent) => {
      if (e.pointerType === 'mouse' && e.button !== 0) {
        return
      }
      stop()
      pressed.current = true
      // a long press whose release never produced a click must not eat the next tap
      fired.current = false
      timer.current = setTimeout(() => {
        timer.current = null
        fired.current = true
        latest.current.onLongPress()
      }, ms)
    },
    [ms, stop],
  )

  const onPointerUp = useCallback(() => {
    stop()
    pressed.current = false
  }, [stop])

  const onPointerLeave = onPointerUp

  // Mobile browsers cancel the pointer when their own long-press callout opens,
  // and that callout can follow the cancel, so `pressed` outlives it until the
  // next pointerdown/up.
  const onPointerCancel = stop

  const onClickHandler = useCallback((e: MouseEvent) => {
    if (fired.current) {
      fired.current = false
      return
    }
    if (e.shiftKey) {
      latest.current.onLongPress()
      return
    }
    latest.current.onClick(e)
  }, [])

  const onContextMenu = useCallback((e: MouseEvent) => {
    if (pressed.current) {
      e.preventDefault()
    }
  }, [])

  return { onPointerDown, onPointerUp, onPointerLeave, onPointerCancel, onClick: onClickHandler, onContextMenu }
}
