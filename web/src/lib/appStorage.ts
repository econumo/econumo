import { isNativeApp, nativePlugin } from './platform'

interface PreferencesPlugin {
  get(o: { key: string }): Promise<{ value: string | null }>
  set(o: { key: string; value: string }): Promise<void>
  remove(o: { key: string }): Promise<void>
}

// WKWebView may evict localStorage under storage pressure; these are the keys
// whose loss logs the user out or strands them on the wrong server. Values are
// mirrored verbatim (storage.ts JSON-encodes non-token keys), so restore is a
// straight copy back.
const MIRRORED_KEYS: readonly string[] = ['token', 'backendHost', 'selfHosted', 'analyticsOptOut']

function prefs(): PreferencesPlugin | null {
  return nativePlugin<PreferencesPlugin>('Preferences')
}

export function mirrorWrite(key: string, value: string | null): void {
  if (!MIRRORED_KEYS.includes(key)) {
    return
  }
  const p = prefs()
  if (!p) {
    return
  }
  // Best-effort mirror; localStorage stays the source of truth.
  void (value === null ? p.remove({ key }).catch(() => {}) : p.set({ key, value }).catch(() => {}))
}

// Runs before first render (the splash screen covers it). localStorage wins
// when it still has the key — the native copy only fills eviction holes.
export async function restoreNativeStorage(): Promise<void> {
  if (!isNativeApp()) {
    return
  }
  const p = prefs()
  if (!p) {
    return
  }
  for (const key of MIRRORED_KEYS) {
    if (localStorage.getItem(key) !== null) {
      continue
    }
    const { value } = await p.get({ key })
    if (value !== null) {
      localStorage.setItem(key, value)
    }
  }
}
