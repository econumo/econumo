import { useCallback, useRef } from 'react'

// Long-press handlers (~500ms) for touch/pointer targets.
export function useLongPress(onLongPress: () => void, delay = 500) {
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const start = useCallback(() => {
    timer.current = setTimeout(onLongPress, delay)
  }, [onLongPress, delay])

  const cancel = useCallback(() => {
    if (timer.current) {
      clearTimeout(timer.current)
      timer.current = null
    }
  }, [])

  return {
    onPointerDown: start,
    onPointerUp: cancel,
    onPointerLeave: cancel,
    onPointerMove: cancel,
  }
}
