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
