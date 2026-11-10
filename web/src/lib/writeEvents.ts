// Writes the server accepted outside a mutation (the pending-writes queue): a
// listener that must see every write cannot rely on the 'invalidate' cache
// event, which fires only for a query not already invalidated.
const listeners = new Set<() => void>()

export function noteWrite(): void {
  for (const listener of listeners) {
    listener()
  }
}

export function onWrite(listener: () => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}
