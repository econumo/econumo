import { useSyncExternalStore } from 'react'
import { isIOS, isNativeApp } from '@/lib/platform'

// Chromium's install prompt; not in the DOM typings.
interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>
}

// 'prompt': the browser can install on request (Chromium). 'ios': iOS has no
// install API, so the user is shown how to add the app from the Share menu.
export type InstallMode = 'prompt' | 'ios'

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
  return isIOS() ? 'ios' : null
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
