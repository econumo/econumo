import { useCallback, useRef } from 'react'

// In-memory only: positions survive route remounts within a session, and a
// full reload deliberately starts every list at the top again.
const positions = new Map<string, number>()

// react-remove-scroll (Radix dialogs AND vaul drawers) marks the body while a
// modal is open. Background scrollers are inert then, so any scroll they
// receive is platform-initiated (e.g. iOS scrolling under a full-screen
// dialog when the keyboard opens) — never the user's position.
const isScrollLocked = () => document.body.hasAttribute('data-scroll-locked')

/**
 * Keeps a scroll container's position across unmounts (compact-layout panes
 * unmount whole sections on navigation) and across modal-induced resets.
 * Attach the returned callback as the container's `ref`.
 */
export function useScrollMemory(key: string): (el: HTMLElement | null) => void {
  const detach = useRef<(() => void) | null>(null)

  return useCallback(
    (el: HTMLElement | null) => {
      detach.current?.()
      detach.current = null
      if (!el) {
        return
      }

      const saved = positions.get(key)
      if (saved !== undefined) {
        el.scrollTop = saved
      }

      const onScroll = () => {
        if (!isScrollLocked()) {
          positions.set(key, el.scrollTop)
        }
      }
      el.addEventListener('scroll', onScroll, { passive: true })

      const observer = new MutationObserver(() => {
        if (!isScrollLocked()) {
          const want = positions.get(key)
          if (want !== undefined && el.scrollTop !== want) {
            el.scrollTop = want
          }
        }
      })
      observer.observe(document.body, { attributes: true, attributeFilter: ['data-scroll-locked'] })

      detach.current = () => {
        el.removeEventListener('scroll', onScroll)
        observer.disconnect()
      }
    },
    [key],
  )
}
