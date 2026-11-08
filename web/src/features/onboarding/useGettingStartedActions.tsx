import { useCallback, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router'
import { RouterPage } from '@/app/router-pages'
import { InstallInstructionsDialog } from '@/features/install/InstallInstructionsDialog'
import { useInstallApp } from '@/features/install/useInstallApp'
import { ImportCsvDialog } from '@/features/transactions/ImportCsvDialog'
import { ImportResultDialog } from '@/features/transactions/ImportResultDialog'
import type { AggregatedImportResult } from '@/features/transactions/importCsv'
import { METRICS, trackEvent } from '@/lib/metrics'
import type { StepKey } from './gettingStarted'
import { markHomeScreenDone } from './useGettingStarted'

export type GettingStartedSurface = 'budget' | 'inbox'

// The dialogs are returned separately so the caller can keep them mounted
// after the cards go away: a CSV import that lands the first transactions
// hides the cards, and its result must still show.
export function useGettingStartedActions(surface: GettingStartedSurface): { open: (key: StepKey) => void; dialogs: ReactNode } {
  const navigate = useNavigate()
  const install = useInstallApp()
  const [importOpen, setImportOpen] = useState(false)
  const [importResult, setImportResult] = useState<AggregatedImportResult | null>(null)
  const { start } = install
  const open = useCallback(
    (key: StepKey) => {
      trackEvent(METRICS.GETTING_STARTED_OPEN, { step: key, surface })
      if (key === 'home_screen') {
        void start('getting_started').then((accepted) => {
          if (accepted) markHomeScreenDone()
        })
      } else if (key === 'apple_wallet') navigate(RouterPage.SETTINGS_APPLE_WALLET)
      else if (key === 'plan') navigate(RouterPage.PLAN)
      else setImportOpen(true)
    },
    [surface, start, navigate],
  )
  // the steps cannot be confirmed from a page, so having read them finishes the step
  const closeInstructions = () => {
    install.closeInstructions()
    markHomeScreenDone()
  }
  const dialogs = (
    <>
      {install.instructions ? <InstallInstructionsDialog mode={install.instructions} open onClose={closeInstructions} /> : null}
      <ImportCsvDialog open={importOpen} onClose={() => setImportOpen(false)} onComplete={setImportResult} />
      <ImportResultDialog open={importResult !== null} result={importResult} onClose={() => setImportResult(null)} />
    </>
  )
  return { open, dialogs }
}
