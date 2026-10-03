import { useEffect, useRef, useState } from 'react'

export const LIST_CHUNK = 100

// The sentinel half of the account page's windowed list: render a chunk and
// grow it as the sentinel nears the scroll container's bottom edge.
export function useWindowed(total: number, resetKey: unknown) {
  const [count, setCount] = useState(LIST_CHUNK)
  const sentinelRef = useRef<HTMLDivElement | null>(null)
  const [prevKey, setPrevKey] = useState(resetKey)
  if (prevKey !== resetKey) {
    setPrevKey(resetKey)
    setCount(LIST_CHUNK)
  }
  const hasMore = count < total

  useEffect(() => {
    const el = sentinelRef.current
    if (!el) {
      return
    }
    // the scroll container must be the root: rootMargin on the viewport root
    // does not expand a scrollable ancestor's clip rect
    const observer = new IntersectionObserver(
      (hits) => {
        if (hits.some((h) => h.isIntersecting)) {
          setCount((c) => c + LIST_CHUNK)
        }
      },
      { root: el.closest('[cmdk-list]') ?? el.parentElement, rootMargin: '600px' },
    )
    observer.observe(el)
    return () => observer.disconnect()
    // re-observe after each growth: the observer only fires on intersection changes
  }, [hasMore, count])

  return { count, hasMore, sentinelRef }
}
