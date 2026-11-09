import { useCallback, useMemo } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useImportSources } from '@/features/imports/queries'
import { isStandalone, useInstallMode } from '@/features/install/installPrompt'
import { useUserData } from '@/features/user/queries'
import { METRICS, trackEvent } from '@/lib/metrics'
import { isIPhone, isMac } from '@/lib/platform'
import { onboardingProgress, markStepsDone, storedStepState, writeStepState, type OnboardingStepKey, type OnboardingStepState, type StoredStepKey } from './onboardingProgress'
import { setHomeScreenState, useHomeScreenState } from './homeScreen'

export type { OnboardingStepKey, OnboardingStepState } from './onboardingProgress'

export interface OnboardingStep {
  key: OnboardingStepKey
  state: OnboardingStepState
}

export interface OnboardingSteps {
  steps: OnboardingStep[]
  current: OnboardingStepKey | null
  allResolved: boolean
  markDone: (key: OnboardingStepKey) => Promise<void>
  // one option write for steps a single action settles together (a CSV import)
  markManyDone: (keys: OnboardingStepKey[]) => Promise<void>
  skip: (key: OnboardingStepKey) => Promise<void>
  ready: boolean
}

export function useOnboardingSteps(): OnboardingSteps {
  const queryClient = useQueryClient()
  const { data: user } = useUserData()
  const sources = useImportSources()
  const installMode = useInstallMode()
  const homeScreen = useHomeScreenState()

  const option = onboardingProgress(user)
  const hasWalletSource = (sources.data ?? []).some((s) => s.provider === 'apple-wallet')
  // A failed source list (routes off with the import providers) only loses
  // the automatic Apple Wallet tick, so its retries are not waited for.
  const ready = user !== undefined && (!sources.isPending || sources.failureCount > 0)

  const steps = useMemo(() => {
    // An install finished on this page leaves no install mode behind; the
    // step stays, ticked, instead of vanishing from under the user.
    const showHomeScreen = installMode !== null || (homeScreen === 'done' && !isStandalone())
    const keys: OnboardingStepKey[] = [
      'import',
      'categories',
      'accounts',
      'plan',
      ...(showHomeScreen ? ['home_screen' as const] : []),
      ...(isIPhone() || isMac() ? ['apple_wallet' as const] : []),
    ]
    return keys.map((key): OnboardingStep => {
      if (key === 'home_screen') return { key, state: homeScreen }
      if (key === 'apple_wallet' && hasWalletSource) return { key, state: 'done' }
      return { key, state: storedStepState(option, key) }
    })
  }, [option, homeScreen, hasWalletSource, installMode])

  const current = steps.find((s) => s.state === 'open')?.key ?? null

  const answer = useCallback(
    async (key: OnboardingStepKey, state: 'done' | 'skipped') => {
      const was = steps.find((s) => s.key === key)?.state
      // Skip on a revisited done step only moves on: the work stays done
      if (was === state || (was === 'done' && state === 'skipped')) return
      trackEvent(state === 'done' ? METRICS.ONBOARDING_STEP_DONE : METRICS.ONBOARDING_STEP_SKIP, { step: key })
      if (key === 'home_screen') {
        setHomeScreenState(state)
        return
      }
      await writeStepState(queryClient, key, state)
    },
    [queryClient, steps],
  )
  const markDone = useCallback((key: OnboardingStepKey) => answer(key, 'done'), [answer])
  const markManyDone = useCallback(
    async (keys: OnboardingStepKey[]) => {
      const stored = keys.filter((k): k is StoredStepKey => k !== 'home_screen' && steps.find((s) => s.key === k)?.state !== 'done')
      stored.forEach((step) => trackEvent(METRICS.ONBOARDING_STEP_DONE, { step }))
      await markStepsDone(queryClient, stored)
    },
    [queryClient, steps],
  )
  const skip = useCallback((key: OnboardingStepKey) => answer(key, 'skipped'), [answer])

  return { steps, current, allResolved: ready && current === null, markDone, markManyDone, skip, ready }
}
