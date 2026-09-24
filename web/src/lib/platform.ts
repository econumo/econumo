// The Capacitor runtime injects window.Capacitor into the WebView; probing it
// keeps web/ free of any Capacitor npm dependency. Plugin call surfaces are
// typed locally at each call site.
interface CapacitorGlobal {
  isNativePlatform?: () => boolean
  Plugins?: Record<string, unknown>
}

declare global {
  interface Window {
    Capacitor?: CapacitorGlobal
  }
}

export function isNativeApp(): boolean {
  return window.Capacitor?.isNativePlatform?.() === true
}

export function nativePlugin<T>(name: string): T | null {
  if (!isNativeApp()) {
    return null
  }
  return (window.Capacitor?.Plugins?.[name] as T | undefined) ?? null
}

// iPadOS 13+ reports itself as a Mac; the touch-point probe tells them apart.
// Used only to decide whether the "Configure on this iPhone" button (a
// shortcuts:// deep link) can work — never for feature gating.
export function isIOS(): boolean {
  const ua = navigator.userAgent
  return /iPhone|iPad|iPod/.test(ua) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)
}
