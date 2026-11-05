import { AppWindow, Check, Download, EllipsisVertical, Share, SquarePlus, type LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { Button } from '@/components/ui/button'
import type { InstructionMode } from './installPrompt'

export function InstallInstructionsDialog({ mode, open, onClose }: { mode: InstructionMode; open: boolean; onClose: () => void }) {
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
  const { intro, steps, hint } = content[mode]
  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose()
      }}
      title={t('install.dialog.title')}
      description={intro}
      footer={
        <Button type="button" className="h-11 w-full" onClick={onClose}>
          {t('install.dialog.close')}
        </Button>
      }
    >
      <ol className="flex flex-col gap-3">
        {steps.map(([Icon, text], i) => (
          <li key={i} className="flex items-center gap-3 text-sm">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-econumo-card">
              <Icon className="size-5 text-econumo-link" />
            </span>
            <span>{text}</span>
          </li>
        ))}
      </ol>
      <p className="mt-4 text-sm text-muted-foreground">{hint}</p>
    </ResponsiveDialog>
  )
}
