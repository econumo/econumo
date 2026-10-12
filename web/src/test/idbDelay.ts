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

// Holds back the `success` of every indexedDB.open by `ms`, as a slow device
// would. Returns a restore function.
export function delayOpen(ms: number): () => void {
  const factory = globalThis.indexedDB
  const real = factory.open
  factory.open = function (this: IDBFactory, ...args: Parameters<IDBFactory['open']>) {
    const req = real.apply(this, args)
    let handler: ((this: IDBRequest, ev: Event) => unknown) | null = null
    Object.defineProperty(req, 'onsuccess', {
      configurable: true,
      get: () => null,
      set: (fn: typeof handler) => { handler = fn },
    })
    req.addEventListener('success', (ev) => {
      setTimeout(() => handler?.call(req, ev), ms)
    })
    return req
  }
  return () => { factory.open = real }
}
