import '@testing-library/jest-dom/vitest'

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
