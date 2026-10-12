// Holds back the `complete` event of matching IndexedDB transactions, as a
// slow or stalled device would; the work itself still runs. `ms: null` never
// completes. Returns a restore function.
export function delayTransactionCompletion(
  ms: number | null,
  match: (stores: string[], mode: IDBTransactionMode | undefined) => boolean = () => true,
): () => void {
  const real = IDBDatabase.prototype.transaction
  const spy = vi.spyOn(IDBDatabase.prototype, 'transaction').mockImplementation(function (
    this: IDBDatabase,
    ...args: Parameters<IDBDatabase['transaction']>
  ) {
    const t = real.apply(this, args)
    const stores = typeof args[0] === 'string' ? [args[0]] : [...args[0]]
    if (!match(stores, args[1])) {
      return t
    }
    let handler: ((this: IDBTransaction, ev: Event) => unknown) | null = null
    Object.defineProperty(t, 'oncomplete', {
      configurable: true,
      get: () => null,
      set: (fn: typeof handler) => { handler = fn },
    })
    t.addEventListener('complete', (ev) => {
      if (ms !== null) {
        setTimeout(() => handler?.call(t, ev), ms)
      }
    })
    return t
  })
  return () => spy.mockRestore()
}
