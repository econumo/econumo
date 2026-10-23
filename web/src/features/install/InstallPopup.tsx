import { useEffect, useState } from 'react'
import { METRICS, trackEvent } from '@/lib/metrics'
import { markInstallPopupShown, shouldShowInstallPopup, useInstallMode } from './installPrompt'
import { InstallInstructionsDialog } from './InstallInstructionsDialog'

// Once per device, the first time a signed-in page opens in iOS Safari: the
// install steps, which also tell anyone who already installed the app to open
// it from the Home Screen. The banner covers every later visit.
export function InstallPopup() {
  const mode = useInstallMode()
  const [open, setOpen] = useState(false)
  useEffect(() => {
    if (mode === 'ios' && shouldShowInstallPopup()) {
      markInstallPopupShown()
      setOpen(true)
      trackEvent(METRICS.INSTALL_APP_OPEN, { source: 'popup', mode })
    }
  }, [mode])
  return open ? <InstallInstructionsDialog open onClose={() => setOpen(false)} /> : null
}
