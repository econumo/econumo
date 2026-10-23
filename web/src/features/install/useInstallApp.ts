import { useCallback, useState } from 'react'
import { METRICS, trackEvent } from '@/lib/metrics'
import { promptInstall, useInstallMode, type InstallMode, type InstructionMode } from './installPrompt'

export type InstallSource = 'settings' | 'banner' | 'popup'

// One entry point for every install surface: Chromium gets its own dialog,
// the menu-only browsers get the steps.
export function useInstallApp(): {
  mode: InstallMode | null
  start: (source: InstallSource) => void
  // the steps to show, while they are open
  instructions: InstructionMode | null
  closeInstructions: () => void
} {
  const mode = useInstallMode()
  const [instructionsOpen, setInstructionsOpen] = useState(false)
  const start = useCallback((source: InstallSource) => {
    if (!mode) {
      return
    }
    trackEvent(METRICS.INSTALL_APP_OPEN, { source, mode })
    if (mode !== 'prompt') {
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
  const instructions = instructionsOpen && mode && mode !== 'prompt' ? mode : null
  return { mode, start, instructions, closeInstructions }
}
