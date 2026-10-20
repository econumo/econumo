import { useCallback, useState } from 'react'
import { METRICS, trackEvent } from '@/lib/metrics'
import { promptInstall, useInstallMode, type InstallMode } from './installPrompt'

export type InstallSource = 'settings' | 'sidebar'

// One entry point for every install surface: Chromium gets its own dialog,
// iOS gets the Share-menu instructions.
export function useInstallApp(): {
  mode: InstallMode | null
  start: (source: InstallSource) => void
  instructionsOpen: boolean
  closeInstructions: () => void
} {
  const mode = useInstallMode()
  const [instructionsOpen, setInstructionsOpen] = useState(false)
  const start = useCallback((source: InstallSource) => {
    if (!mode) {
      return
    }
    trackEvent(METRICS.INSTALL_APP_OPEN, { source, mode })
    if (mode === 'ios') {
      setInstructionsOpen(true)
      return
    }
    void promptInstall().then((accepted) => {
      if (accepted) {
        trackEvent(METRICS.INSTALL_APP_ACCEPT, { source })
      }
    })
  }, [mode])
  const closeInstructions = useCallback(() => setInstructionsOpen(false), [])
  return { mode, start, instructionsOpen, closeInstructions }
}
