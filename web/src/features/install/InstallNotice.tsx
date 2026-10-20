import { useState } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { METRICS, trackEvent } from '@/lib/metrics'
import { dismissInstallNotice, isInstallNoticeDismissed } from './installPrompt'
import { useInstallApp } from './useInstallApp'
import { InstallInstructionsDialog } from './InstallInstructionsDialog'

// Sits in the sidebar's notice slot next to UpdateNotice; one dismissal hides
// it for good (Settings keeps the "Install the app" row).
export function InstallNotice() {
  const { t } = useTranslation()
  const { mode, start, instructionsOpen, closeInstructions } = useInstallApp()
  const [dismissed, setDismissed] = useState(isInstallNoticeDismissed)
  return (
    <>
      {mode && !dismissed ? (
        <div className="mx-3 mb-2 flex items-center gap-2 rounded-md bg-accent px-3 py-2 text-xs text-muted-foreground">
          <button
            type="button"
            className="min-w-0 flex-1 truncate text-left font-medium hover:text-foreground"
            onClick={() => start('sidebar')}
          >
            {t('install.notice')}
          </button>
          <button
            type="button"
            aria-label={t('install.dismiss')}
            className="shrink-0 hover:text-foreground"
            onClick={() => {
              dismissInstallNotice()
              setDismissed(true)
              trackEvent(METRICS.INSTALL_NOTICE_DISMISS)
            }}
          >
            <X className="size-3.5" />
          </button>
        </div>
      ) : null}
      <InstallInstructionsDialog open={instructionsOpen} onClose={closeInstructions} />
    </>
  )
}
