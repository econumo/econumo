import { useCallback, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useImportSources } from '@/features/imports/queries'
import { isStandalone, useInstallMode } from '@/features/install/installPrompt'
import { useTransactions } from '@/features/transactions/queries'
import { gettingStartedOption, useUserData } from '@/features/user/queries'
import { useIsCompact } from '@/hooks/useIsCompact'
import { METRICS, trackEvent } from '@/lib/metrics'
import { isIOS } from '@/lib/platform'
import {
  hasRealTransactions,
  dismissGettingStarted,
  markGettingStartedDone,
  storedStepState,
  type StepKey,
  type StepState,
} from './gettingStarted'

export type { StepKey } from './gettingStarted'

export const HOME_SCREEN_KEY = 'econumo-getting-started-home-screen'

export interface GettingStartedStep {
  key: StepKey
  done: boolean
  dismissed: boolean
}

export interface GettingStarted {
  active: boolean
  /** the budget page's cards: this device's own steps */
  steps: GettingStartedStep[]
  /** the Inbox list: this device's steps plus plan and import on every device */
  inboxSteps: GettingStartedStep[]
  hasData: boolean
  markDone: (key: StepKey) => void
  dismiss: (key: StepKey) => void
}

export function isOpenStep(step: GettingStartedStep): boolean {
  return !step.done && !step.dismissed
}

function homeScreenState(): StepState {
  if (isStandalone()) return 'done'
  const stored = localStorage.getItem(HOME_SCREEN_KEY)
  return stored === 'done' || stored === 'dismissed' ? stored : 'open'
}

export function useGettingStarted(): GettingStarted {
  const queryClient = useQueryClient()
  const { data: user } = useUserData()
  const { data: transactions } = useTransactions()
  const { data: sources } = useImportSources()
  const isCompact = useIsCompact()
  // a browser that cannot install from a page has nothing for the card to open
  const installable = useInstallMode() !== null
  const [homeScreen, setHomeScreen] = useState(homeScreenState)

  const option = gettingStartedOption(user)
  const active = option !== null
  const hasWalletSource = (sources ?? []).some((s) => s.provider === 'apple-wallet')

  const { steps, inboxSteps } = useMemo(() => {
    const state = (key: StepKey): StepState => {
      if (key === 'home_screen') return homeScreen
      if (key === 'apple_wallet' && hasWalletSource) return 'done'
      return storedStepState(option, key)
    }
    const step = (key: StepKey): GettingStartedStep => ({ key, done: state(key) === 'done', dismissed: state(key) === 'dismissed' })
    const phoneKeys: StepKey[] = [
      ...(installable || homeScreen === 'done' ? ['home_screen' as const] : []),
      ...(isIOS() ? ['apple_wallet' as const] : []),
    ]
    const cardKeys: StepKey[] = isCompact ? phoneKeys : ['plan', 'import']
    const inboxKeys = [...new Set<StepKey>([...cardKeys, 'plan', 'import'])]
    return { steps: cardKeys.map(step), inboxSteps: inboxKeys.map(step) }
  }, [option, homeScreen, hasWalletSource, installable, isCompact])

  // Unknown counts as data, so the cards never cover a budget whose
  // transactions are still loading.
  const hasData = transactions === undefined ? true : hasRealTransactions(transactions)

  const markDone = useCallback(
    (key: StepKey) => {
      if (key === 'home_screen') {
        localStorage.setItem(HOME_SCREEN_KEY, 'done')
        setHomeScreen('done')
        return
      }
      markGettingStartedDone(queryClient, key)
    },
    [queryClient],
  )

  const dismiss = useCallback(
    (key: StepKey) => {
      trackEvent(METRICS.GETTING_STARTED_DISMISS, { step: key })
      if (key === 'home_screen') {
        localStorage.setItem(HOME_SCREEN_KEY, 'dismissed')
        setHomeScreen('dismissed')
        return
      }
      dismissGettingStarted(queryClient, key)
    },
    [queryClient],
  )

  return { active, steps, inboxSteps, hasData, markDone, dismiss }
}
