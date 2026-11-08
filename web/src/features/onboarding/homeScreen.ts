import { useEffect, useSyncExternalStore } from 'react'
import { isAppInstalled, isStandalone, subscribeInstallState } from '@/features/install/installPrompt'
import type { OnboardingStepState } from './gettingStarted'

export const HOME_SCREEN_KEY = 'econumo-getting-started-home-screen'

// Per device and shared by every hook instance: the install dialog that
// finishes the step may be opened from another component.
const listeners = new Set<() => void>()

function homeScreenState(): OnboardingStepState {
  if (isStandalone() || isAppInstalled()) return 'done'
  const stored = localStorage.getItem(HOME_SCREEN_KEY)
  if (stored === 'done') return 'done'
  // 'dismissed' is what the first version of the flow wrote
  return stored === 'skipped' || stored === 'dismissed' ? 'skipped' : 'open'
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  const unsubscribeInstall = subscribeInstallState(listener)
  return () => {
    listeners.delete(listener)
    unsubscribeInstall()
  }
}

export function setHomeScreenState(state: Exclude<OnboardingStepState, 'open'>): void {
  localStorage.setItem(HOME_SCREEN_KEY, state)
  listeners.forEach((l) => l())
}

export function markHomeScreenDone(): void {
  setHomeScreenState('done')
}

export function useHomeScreenState(): OnboardingStepState {
  const state = useSyncExternalStore(subscribe, homeScreenState)
  // an install seen only in this page's memory is written down for the next launch
  useEffect(() => {
    if (state === 'done' && isAppInstalled() && localStorage.getItem(HOME_SCREEN_KEY) !== 'done') {
      localStorage.setItem(HOME_SCREEN_KEY, 'done')
    }
  }, [state])
  return state
}
