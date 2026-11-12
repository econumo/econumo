import '@testing-library/jest-dom/vitest'
import axios from 'axios'

// Under jsdom axios picks its XHR adapter, and msw's XHR interceptor never
// delivers the response when the request body is FormData (the promise hangs
// forever). Route axios through the fetch adapter, which msw handles fine.
// Must run before api/client.ts calls axios.create(), which snapshots defaults.
//
// A write still unanswered when its test ends would time out 10 s later and queue
// itself, and the queue is module state: the change would land in whichever test is
// running by then and show up in its cells. So a test's writes end with it.
const writesInFlight = new Map<AbortController, Promise<unknown>>()
axios.defaults.adapter = (config) => {
  const fetchAdapter = axios.getAdapter('fetch')
  if (config.method !== 'post') {
    return fetchAdapter(config)
  }
  const controller = new AbortController()
  const signal = config.signal ? AbortSignal.any([config.signal as AbortSignal, controller.signal]) : controller.signal
  const response = fetchAdapter({ ...config, signal })
  writesInFlight.set(controller, response.catch(() => {}).finally(() => writesInFlight.delete(controller)))
  return response
}
const realSetTimeout = globalThis.setTimeout
afterEach(async () => {
  for (const controller of writesInFlight.keys()) {
    controller.abort()
  }
  await Promise.all(writesInFlight.values())
  // what the failed requests' callers do with it is over too, before the next test resets the queue
  await new Promise((resolve) => realSetTimeout(resolve))
})

// Node >= 25 defines an experimental global `localStorage` (unusable without
// --localstorage-file), so vitest's jsdom environment skips copying jsdom's
// implementation onto globalThis. Rebind the real jsdom storages explicitly.
const jsdom = (globalThis as { jsdom?: { window: Window } }).jsdom
if (jsdom) {
  for (const key of ['localStorage', 'sessionStorage'] as const) {
    Object.defineProperty(globalThis, key, {
      value: jsdom.window[key],
      writable: true,
      configurable: true,
    })
  }
}

// jsdom has no scrollIntoView; cmdk (Command) calls it on selection changes.
if (typeof Element !== 'undefined' && !Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => {}
}

// jsdom has no IntersectionObserver; the windowed transaction list uses one.
if (typeof globalThis.IntersectionObserver === 'undefined') {
  globalThis.IntersectionObserver = class IntersectionObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof globalThis.IntersectionObserver
}

// jsdom has no ResizeObserver; Radix primitives (checkbox, etc.) expect one.
if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = class ResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
}

// jsdom has no Pointer Capture API; Radix Select's trigger calls it on pointerdown.
if (typeof Element !== 'undefined' && !Element.prototype.hasPointerCapture) {
  Element.prototype.hasPointerCapture = () => false
  Element.prototype.setPointerCapture = () => {}
  Element.prototype.releasePointerCapture = () => {}
}

// jsdom ships neither IndexedDB nor a full Web Crypto; the import credential
// key lives in both. Node's webcrypto is spec-compliant for what we use.
import 'fake-indexeddb/auto'
if (typeof globalThis.crypto?.subtle === 'undefined') {
  const { webcrypto } = await import('node:crypto')
  Object.defineProperty(globalThis, 'crypto', { value: webcrypto, writable: true, configurable: true })
}

// Imported dynamically so the storage rebind above runs first (i18n reads the
// persisted locale from localStorage at init).
await import('@/app/i18n')

// Transactions now persist to IndexedDB on every sync, so a test must not see
// the buckets a previous test in the same file left behind.
const { IDBFactory } = await import('fake-indexeddb')
const { resetDbForTests } = await import('@/lib/db')
const { resetTransactionStoreForTests } = await import('@/lib/transactionStore')
const { resetPendingWritesForTests } = await import('@/features/pendingWrites/store')
const { resetIconSuggestionsForTests } = await import('@/features/iconSuggestion/answers')
beforeEach(() => {
  resetIconSuggestionsForTests()
  globalThis.indexedDB = new IDBFactory()
  resetDbForTests()
  resetTransactionStoreForTests()
  resetPendingWritesForTests()
})

// jsdom leaves isSecureContext undefined; the app runs on https or localhost,
// where browsers report a secure page (SimpleFIN needs it for WebCrypto)
Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true })
