import { useTranslation } from 'react-i18next'
import { CalendarRange, FileUp, Smartphone, Wallet, X, type LucideIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { StepKey } from './gettingStarted'
import { isOpenStep, type GettingStartedStep } from './useGettingStarted'

const ICONS: Record<StepKey, LucideIcon> = {
  home_screen: Smartphone,
  apple_wallet: Wallet,
  plan: CalendarRange,
  import: FileUp,
}

function StepText({ step }: { step: StepKey }) {
  const { t } = useTranslation()
  const Icon = ICONS[step]
  return (
    <div className="flex min-w-0 flex-1 items-start gap-3">
      <Icon className="mt-0.5 size-5 shrink-0 text-muted-foreground" aria-hidden />
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="text-sm">{t(`onboarding.cards.${step}.title`)}</span>
        <span className="text-xs text-muted-foreground">{t(`onboarding.cards.${step}.description`)}</span>
      </div>
    </div>
  )
}

export function GettingStartedCards({
  steps,
  onOpen,
  onDismiss,
}: {
  steps: GettingStartedStep[]
  onOpen: (key: StepKey) => void
  onDismiss: (key: StepKey) => void
}) {
  const { t } = useTranslation()
  const open = steps.filter(isOpenStep)
  return (
    <section className="mx-auto flex w-full max-w-xl flex-col gap-3 py-2 text-left" data-testid="getting-started">
      <div className="flex flex-col gap-0.5 px-1">
        <h2 className="text-lg">{t('onboarding.cards.title')}</h2>
        <p className="text-sm text-muted-foreground">{t('onboarding.cards.description')}</p>
      </div>
      {open.map((s) => (
        <div key={s.key} className="flex flex-col gap-3 rounded-lg bg-econumo-card px-4 py-3">
          <StepText step={s.key} />
          <div className="flex justify-end gap-2">
            <Button type="button" variant="ghost" size="sm" onClick={() => onDismiss(s.key)}>
              {t('onboarding.cards.skip')}
            </Button>
            <Button type="button" size="sm" onClick={() => onOpen(s.key)}>
              {t(`onboarding.cards.${s.key}.action`)}
            </Button>
          </div>
        </div>
      ))}
    </section>
  )
}

export function NextStepItem({ step, onOpen, onDismiss }: { step: StepKey; onOpen: () => void; onDismiss: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex items-center gap-2 rounded-lg bg-econumo-card pr-2 hover:bg-econumo-hover">
      <button type="button" onClick={onOpen} className="flex min-w-0 flex-1 px-4 py-3 text-left">
        <StepText step={step} />
      </button>
      <Button type="button" variant="ghost" size="icon" aria-label={t('onboarding.cards.dismiss', { step: t(`onboarding.cards.${step}.title`) })} onClick={onDismiss}>
        <X className="size-4" />
      </Button>
    </div>
  )
}
