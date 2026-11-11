import { useState } from 'react'

// True from the first render where `ready` held, and for good after it. A step decides what to
// offer (default categories, the Cash row) from its list once; the list cached before a CSV
// import is stale, so the step waits for the refetch, but a later background refetch (the AI
// refresh) must not unmount the form the user is filling in.
export function useSettled(ready: boolean): boolean {
  const [settled, setSettled] = useState(ready)
  if (ready && !settled) setSettled(true)
  return settled || ready
}
