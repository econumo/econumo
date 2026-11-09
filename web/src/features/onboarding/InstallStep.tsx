import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { InstallSteps } from '@/features/install/InstallSteps'
import { useInstallApp } from '@/features/install/useInstallApp'
import type { StepBodyProps } from './OnboardingPage'

export function InstallStep({ onDone, onSkip }: StepBodyProps) {
  const { t } = useTranslation()
  const { mode, start } = useInstallApp()
  const [pending, setPending] = useState(false)
  const run = (action: () => Promise<void>) => () => {
    setPending(true)
    void action().finally(() => setPending(false))
  }
  const install = async () => {
    if (await start('onboarding')) await onDone()
  }
  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-2xl font-normal">{t('onboarding.install_step.heading')}</h2>
      <p className="text-sm text-muted-foreground">{t('onboarding.install_step.text')}</p>
      {mode !== null && mode !== 'prompt' ? (
        <div className="mt-4 rounded-xl border bg-card p-3 [&_ol]:gap-0 [&_ol>li]:px-1 [&_ol>li]:py-2 [&_ol>li+li]:border-t [&_ol>li+li]:border-border/60 [&_li>span:first-child]:bg-primary/10 [&_li_svg]:text-muted-foreground">
          <InstallSteps mode={mode} />
        </div>
      ) : null}
      <div className="mt-6 flex items-center justify-between">
        <Button type="button" variant="ghost" disabled={pending} onClick={run(onSkip)}>
          {t('onboarding.install_step.skip')}
        </Button>
        {mode === 'prompt' ? (
          <Button type="button" disabled={pending} onClick={run(install)}>
            {t('onboarding.install_step.install')}
          </Button>
        ) : (
          <Button type="button" disabled={pending} onClick={run(() => onDone())}>
            {t('onboarding.install_step.done')}
          </Button>
        )}
      </div>
    </section>
  )
}
