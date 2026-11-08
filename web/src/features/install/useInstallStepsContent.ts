import { AppWindow, Check, Download, EllipsisVertical, Share, SquarePlus, type LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { InstructionMode } from './installPrompt'

export function useInstallStepsContent(mode: InstructionMode): { intro: string; steps: [LucideIcon, string][]; hint: string } {
  const { t } = useTranslation()
  const content: Record<InstructionMode, { intro: string; steps: [LucideIcon, string][]; hint: string }> = {
    ios: {
      intro: t('install.dialog.intro'),
      steps: [
        [Share, t('install.dialog.step_share')],
        [SquarePlus, t('install.dialog.step_add')],
        [Check, t('install.dialog.step_confirm')],
      ],
      hint: t('install.dialog.installed_hint'),
    },
    android_firefox: {
      intro: t('install.dialog.android_firefox.intro'),
      steps: [
        [EllipsisVertical, t('install.dialog.android_firefox.step_menu')],
        [Download, t('install.dialog.android_firefox.step_install')],
        [Check, t('install.dialog.android_firefox.step_confirm')],
      ],
      hint: t('install.dialog.android_firefox.installed_hint'),
    },
    macos_safari: {
      intro: t('install.dialog.macos.intro'),
      steps: [
        [AppWindow, t('install.dialog.macos.step_file')],
        [SquarePlus, t('install.dialog.macos.step_add')],
        [Check, t('install.dialog.macos.step_confirm')],
      ],
      hint: t('install.dialog.macos.installed_hint'),
    },
  }
  return content[mode]
}
