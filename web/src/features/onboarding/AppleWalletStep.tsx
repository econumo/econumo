import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { RouterPage } from '@/app/router-pages'
import { Button } from '@/components/ui/button'
import type { StepBodyProps } from './OnboardingPage'

export function AppleWalletStep({ onSkip }: StepBodyProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [pending, setPending] = useState(false)
  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-2xl font-normal">{t('onboarding.wallet_step.heading')}</h2>
      <p className="text-sm text-muted-foreground">{t('onboarding.wallet_step.text')}</p>
      <button
        type="button"
        className="mt-4 flex items-center justify-center rounded-xl border-2 border-dashed border-primary/40 bg-primary/5 p-6 text-sm text-muted-foreground hover:bg-primary/10"
        onClick={() => navigate(RouterPage.SETTINGS_APPLE_WALLET)}
      >
        {t('onboarding.wallet_step.tile')}
      </button>
      <div className="mt-6 flex items-center justify-between">
        <Button
          type="button"
          variant="ghost"
          disabled={pending}
          onClick={() => {
            setPending(true)
            void onSkip().finally(() => setPending(false))
          }}
        >
          {t('onboarding.wallet_step.skip')}
        </Button>
        <Button type="button" disabled={pending} onClick={() => navigate(RouterPage.SETTINGS_APPLE_WALLET)}>
          {t('onboarding.wallet_step.setup')}
        </Button>
      </div>
    </section>
  )
}
