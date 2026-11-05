import { useSyncExternalStore } from 'react'
import { isIOS, isNativeApp } from '@/lib/platform'

// Chromium's install prompt; not in the DOM typings.
interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>
}

// 'prompt': the browser can install on request (Chromium). The others can
// install only from a browser menu and have no install API, so the user is
// shown the steps: iOS from the Share menu, macOS Safari 17+ with File → Add
// to Dock, Firefox for Android from its ⋮ menu.
export type InstallMode = 'prompt' | InstructionMode
export type InstructionMode = 'ios' | 'macos_safari' | 'android_firefox'

const DISMISSED_KEY = 'econumo.install-notice-dismissed'
const POPUP_SHOWN_KEY = 'econumo.install-popup-shown'

let deferredPrompt: BeforeInstallPromptEvent | null = null
let installed = false
const listeners = new Set<() => void>()

function notify(): void {
  listeners.forEach((l) => l())
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

// The event fires once, early, possibly before React mounts, so the listener
// is installed from main.tsx and the event is kept for a later click.
export function listenForInstallPrompt(): void {
  window.addEventListener('beforeinstallprompt', (e) => {
    e.preventDefault()
    deferredPrompt = e as BeforeInstallPromptEvent
    notify()
  })
  window.addEventListener('appinstalled', () => {
    deferredPrompt = null
    installed = true
    notify()
  })
}

export function isStandalone(): boolean {
  return (
    window.matchMedia?.('(display-mode: standalone)').matches === true ||
    (navigator as Navigator & { standalone?: boolean }).standalone === true
  )
}

export function installMode(): InstallMode | null {
  if (installed || isNativeApp() || isStandalone()) {
    return null
  }
  if (deferredPrompt) {
    return 'prompt'
  }
  if (isIOS()) {
    return 'ios'
  }
  const ua = navigator.userAgent
  if (/Android/.test(ua) && /Firefox\//.test(ua)) {
    return 'android_firefox'
  }
  // Add to Dock arrived in Safari 17; every other macOS browser names itself
  // in the user agent next to "Safari"
  const safari = /Macintosh/.test(ua) && /Version\/(\d+)[.\d]* Safari\//.exec(ua)
  if (safari && !/Chrome|Chromium|Edg|OPR|Firefox/.test(ua) && Number(safari[1]) >= 17) {
    return 'macos_safari'
  }
  return null
}

export function useInstallMode(): InstallMode | null {
  return useSyncExternalStore(subscribe, installMode)
}

// Resolves true when the user accepted the browser's install dialog.
export async function promptInstall(): Promise<boolean> {
  const event = deferredPrompt
  if (!event) {
    return false
  }
  // a prompt event can be used once
  deferredPrompt = null
  notify()
  await event.prompt()
  const { outcome } = await event.userChoice
  return outcome === 'accepted'
}

export function isInstallNoticeDismissed(): boolean {
  try {
    return localStorage.getItem(DISMISSED_KEY) === '1'
  } catch {
    return false
  }
}

export function dismissInstallNotice(): void {
  try {
    localStorage.setItem(DISMISSED_KEY, '1')
  } catch {
    // storage unavailable: the notice comes back on the next launch
  }
}

// The one-time popup at the first signed-in visit on iOS Safari.
export function shouldShowInstallPopup(): boolean {
  try {
    return localStorage.getItem(POPUP_SHOWN_KEY) !== '1' && !isInstallNoticeDismissed()
  } catch {
    return false
  }
}

export function markInstallPopupShown(): void {
  try {
    localStorage.setItem(POPUP_SHOWN_KEY, '1')
  } catch {
    // storage unavailable: shouldShowInstallPopup already said no
  }
}

export function resetInstallStateForTests(): void {
  deferredPrompt = null
  installed = false
}
