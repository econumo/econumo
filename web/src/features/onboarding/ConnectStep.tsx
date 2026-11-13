import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, Copy, ExternalLink } from 'lucide-react'
import { useNavigate } from 'react-router'
import { RouterPage } from '@/app/router-pages'
import { SIMPLEFIN_BRIDGE_URL } from '@/features/imports/simplefinBridge'
import { Button } from '@/components/ui/button'
import { useOnline } from '@/hooks/useOnline'
import { copyText } from '@/lib/clipboard'
import { mcpUrl } from '@/lib/config'
import { METRICS, trackEvent } from '@/lib/metrics'
import type { StepBodyProps } from './OnboardingPage'
import { StepFooter } from './StepFooter'
import { connectOptions, type ConnectOption } from './useOnboardingSteps'

const SETTINGS: Record<Exclude<ConnectOption, 'mcp'>, string> = {
  apple_wallet: RouterPage.SETTINGS_APPLE_WALLET,
  simplefin: RouterPage.SETTINGS_SIMPLEFIN,
}

export function ConnectStep({ onDone, coreStepsDone, onCompleteSetup }: StepBodyProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const online = useOnline()
  const [pending, setPending] = useState(false)
  const [copied, setCopied] = useState(false)
  const options = connectOptions()
  const address = mcpUrl()

  const run = (action: () => Promise<void>) => {
    setPending(true)
    void action().finally(() => setPending(false))
  }

  // Once Categories and Accounts are done, leaving for a settings page wraps the guide up first,
  // as the step is the guide's last; before that the user comes back to finish them.
  const open = (option: Exclude<ConnectOption, 'mcp'>) => {
    trackEvent(METRICS.ONBOARDING_CONNECT_OPEN, { option })
    if (!coreStepsDone || !onCompleteSetup) {
      navigate(SETTINGS[option])
      return
    }
    run(() => onCompleteSetup(SETTINGS[option], 'connect'))
  }

  const copy = () => {
    trackEvent(METRICS.ONBOARDING_CONNECT_OPEN, { option: 'mcp' })
    void copyText(address).then(setCopied)
  }

  const finish = () => run(() => (onCompleteSetup ? onCompleteSetup(RouterPage.BUDGET, 'connect') : onDone()))

  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-2xl font-normal">{t('onboarding.connect_step.heading')}</h2>
      <p className="text-sm text-muted-foreground">{t('onboarding.connect_step.text')}</p>
      <div className="mt-4 flex flex-col gap-3">
        {options.map((option) => (
          <div key={option} data-testid={`connect-${option}`} className="flex flex-col gap-1 rounded-xl border bg-card p-4">
            <h3 className="text-base font-normal">{t(`onboarding.connect_step.${option}.title`)}</h3>
            <p className="text-sm text-muted-foreground">{t(`onboarding.connect_step.${option}.text`)}</p>
            {option === 'mcp' ? (
              <div className="mt-2 flex items-center gap-2 rounded-lg bg-econumo-card px-3 py-2">
                <code className="min-w-0 flex-1 break-all text-xs">{address}</code>
                <Button type="button" size="sm" variant="secondary" onClick={copy}>
                  {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
                  {copied ? t('onboarding.connect_step.copied') : t('onboarding.connect_step.copy')}
                </Button>
              </div>
            ) : (
              <div className="flex flex-wrap items-center gap-x-6 gap-y-1">
                <Button type="button" variant="link" className="h-auto p-0" disabled={pending} onClick={() => open(option)}>
                  {t(`onboarding.connect_step.${option}.link`)}
                </Button>
                {option === 'simplefin' ? (
                  <a
                    href={SIMPLEFIN_BRIDGE_URL}
                    target="_blank"
                    rel="noopener"
                    className="inline-flex items-center gap-1.5 text-sm text-econumo-link underline-offset-2 hover:underline"
                    onClick={() => trackEvent(METRICS.ONBOARDING_CONNECT_OPEN, { option: 'simplefin_site' })}
                  >
                    {t('onboarding.connect_step.simplefin.site')}
                    <ExternalLink className="size-3.5" />
                  </a>
                ) : null}
              </div>
            )}
          </div>
        ))}
      </div>
      <p className="mt-2 text-sm text-muted-foreground">{t('onboarding.connect_step.later')}</p>
      <StepFooter>
        <span />
        <Button type="button" disabled={pending || !online} onClick={finish}>
          {t('onboarding.connect_step.finish')}
        </Button>
      </StepFooter>
    </section>
  )
}
