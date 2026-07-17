import '@testing-library/jest-dom/vitest'
import axios from 'axios'

// Under jsdom axios picks its XHR adapter, and msw's XHR interceptor never
// delivers the response when the request body is FormData (the promise hangs
// forever). Route axios through the fetch adapter, which msw handles fine.
// Must run before api/client.ts calls axios.create(), which snapshots defaults.
axios.defaults.adapter = 'fetch'

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

// Imported dynamically so the storage rebind above runs first (i18n reads the
// persisted locale from localStorage at init).
await import('@/app/i18n')
