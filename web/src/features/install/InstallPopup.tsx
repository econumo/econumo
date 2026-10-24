import { useEffect, useState } from 'react'
import { METRICS, trackEvent } from '@/lib/metrics'
import { markInstallPopupShown, shouldShowInstallPopup, useInstallMode } from './installPrompt'
import { InstallInstructionsDialog } from './InstallInstructionsDialog'

// Once per device, the first time a signed-in page opens in a browser that
// installs only from a menu: the install steps, which also tell anyone who
// already installed the app to open it from there instead. The banner covers every later visit.
export function InstallPopup() {
  const mode = useInstallMode()
  const [open, setOpen] = useState(false)
  useEffect(() => {
    if (mode && mode !== 'prompt' && shouldShowInstallPopup()) {
      markInstallPopupShown()
      setOpen(true)
      trackEvent(METRICS.INSTALL_APP_OPEN, { source: 'popup', mode })
    }
  }, [mode])
  return open && mode && mode !== 'prompt' ? <InstallInstructionsDialog mode={mode} open onClose={() => setOpen(false)} /> : null
}
