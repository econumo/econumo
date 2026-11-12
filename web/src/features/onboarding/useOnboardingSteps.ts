import { useCallback, useMemo } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useImportSources } from '@/features/imports/queries'
import { isStandalone, useInstallMode } from '@/features/install/installPrompt'
import { useUserData } from '@/features/user/queries'
import { METRICS, trackEvent } from '@/lib/metrics'
import { isSimpleFINImportEnabled, mcpUrl } from '@/lib/config'
import { isIPhone, isMac } from '@/lib/platform'
import { onboardingProgress, storedStepState, writeStepState, type OnboardingStepKey, type OnboardingStepState } from './onboardingProgress'
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
  skip: (key: OnboardingStepKey) => Promise<void>
  ready: boolean
}

export type ConnectOption = 'apple_wallet' | 'simplefin' | 'mcp'

// What the Connect step offers on this device and server; with none it is left out.
export function connectOptions(): ConnectOption[] {
  return [
    ...(isIPhone() || isMac() ? ['apple_wallet' as const] : []),
    // the browser encrypts the SimpleFIN access URL, which needs a secure page
    ...(isSimpleFINImportEnabled() && window.isSecureContext ? ['simplefin' as const] : []),
    ...(mcpUrl() !== '' ? ['mcp' as const] : []),
  ]
}

export function useOnboardingSteps(): OnboardingSteps {
  const queryClient = useQueryClient()
  const { data: user } = useUserData()
  const sources = useImportSources()
  const installMode = useInstallMode()
  const homeScreen = useHomeScreenState()

  const option = onboardingProgress(user)
  const hasSource = (sources.data ?? []).length > 0
  // A failed source list only loses the automatic Connect tick, so its retries are not waited for.
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
      ...(connectOptions().length > 0 ? ['connect' as const] : []),
    ]
    return keys.map((key): OnboardingStep => {
      if (key === 'home_screen') return { key, state: homeScreen }
      if (key === 'connect' && hasSource) return { key, state: 'done' }
      return { key, state: storedStepState(option, key) }
    })
  }, [option, homeScreen, hasSource, installMode])

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
  const skip = useCallback((key: OnboardingStepKey) => answer(key, 'skipped'), [answer])

  return { steps, current, allResolved: ready && current === null, markDone, skip, ready }
}
