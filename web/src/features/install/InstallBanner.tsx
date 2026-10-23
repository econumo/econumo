import { useState } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { METRICS, trackEvent } from '@/lib/metrics'
import { dismissInstallNotice, isInstallNoticeDismissed } from './installPrompt'
import { useInstallApp } from './useInstallApp'
import { InstallInstructionsDialog } from './InstallInstructionsDialog'

// Top of every page, signed in or not, until it is dismissed or the app runs
// installed. One dismissal hides it for good on this device; Settings keeps
// the "Install the app" row.
export function InstallBanner() {
  const { t } = useTranslation()
  const { mode, start, instructionsOpen, closeInstructions } = useInstallApp()
  const [dismissed, setDismissed] = useState(isInstallNoticeDismissed)
  return (
    <>
      {mode && !dismissed ? (
        <div className="flex items-center gap-3 bg-primary/10 px-4 py-2 text-sm text-econumo-link">
          <span className="min-w-0 flex-1">{t('install.notice')}</span>
          <Button type="button" size="sm" variant="outline" className="shrink-0" onClick={() => start('banner')}>
            {t('install.cta')}
          </Button>
          <button
            type="button"
            aria-label={t('install.dismiss')}
            className="shrink-0 hover:opacity-70"
            onClick={() => {
              dismissInstallNotice()
              setDismissed(true)
              trackEvent(METRICS.INSTALL_NOTICE_DISMISS)
            }}
          >
            <X className="size-4" />
          </button>
        </div>
      ) : null}
      {/* mounted on demand: the banner sits on every page, the dialog is rare */}
      {instructionsOpen ? <InstallInstructionsDialog open onClose={closeInstructions} /> : null}
    </>
  )
}
