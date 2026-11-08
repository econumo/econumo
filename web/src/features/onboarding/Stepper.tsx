import { Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useIsCompact } from '@/hooks/useIsCompact'
import type { OnboardingStep, OnboardingStepKey } from './useOnboardingSteps'

type ItemState = 'done' | 'skipped' | 'current' | 'open'

interface StepperProps {
  steps: OnboardingStep[]
  current: OnboardingStepKey
  onSelect: (key: OnboardingStepKey) => void
}

const itemState = (step: OnboardingStep, current: OnboardingStepKey): ItemState =>
  step.key === current ? 'current' : step.state

const DOT: Record<ItemState, string> = {
  done: 'border-primary bg-primary text-primary-foreground',
  skipped: 'border-dashed border-border bg-background text-muted-foreground',
  current: 'border-primary bg-background text-primary',
  open: 'border-border bg-background text-muted-foreground',
}

const SEGMENT: Record<ItemState, string> = {
  done: 'bg-primary',
  skipped: 'bg-muted-foreground/30',
  current: 'bg-primary/35',
  open: 'bg-border',
}

export function Stepper({ steps, current, onSelect }: StepperProps) {
  const { t } = useTranslation()
  const isCompact = useIsCompact()
  const label = (key: OnboardingStepKey) => t(`onboarding.steps_nav.${key}`)
  // the dot and bar colours carry done/skipped for the eye; screen readers get it in words
  const stateWord = (step: OnboardingStep) => (step.state === 'open' ? null : t(`onboarding.steps_nav.state_${step.state}`))
  const spoken = (step: OnboardingStep) => [label(step.key), stateWord(step)].filter(Boolean).join(', ')

  if (isCompact) {
    const position = steps.findIndex((s) => s.key === current) + 1
    return (
      <nav aria-label={t('onboarding.steps_nav.title')}>
        <p className="text-sm text-muted-foreground">{t('onboarding.steps_nav.progress', { current: position, total: steps.length })}</p>
        <div className="mt-1 flex gap-1.5">
          {steps.map((step) => {
            const state = itemState(step, current)
            return (
              <button
                key={step.key}
                type="button"
                aria-label={spoken(step)}
                aria-current={state === 'current' ? 'step' : undefined}
                data-state={state}
                onClick={() => onSelect(step.key)}
                className="flex h-6 flex-1 items-center"
              >
                <span className={`h-1 w-full rounded-full ${SEGMENT[state]}`} />
              </button>
            )
          })}
        </div>
      </nav>
    )
  }

  return (
    <nav aria-label={t('onboarding.steps_nav.title')}>
      <ol className="flex">
        {steps.map((step, index) => {
          const state = itemState(step, current)
          return (
            <li key={step.key} className="relative flex flex-1 justify-center">
              {index < steps.length - 1 ? (
                <span
                  aria-hidden
                  className={`absolute top-3.5 right-[calc(-50%+1.125rem)] left-[calc(50%+1.125rem)] h-0.5 ${step.state === 'done' ? 'bg-primary/30' : 'bg-border'}`}
                />
              ) : null}
              <button
                type="button"
                aria-current={state === 'current' ? 'step' : undefined}
                data-state={state}
                onClick={() => onSelect(step.key)}
                className={`flex flex-col items-center gap-1.5 rounded-md px-1 text-center text-xs ${state === 'current' ? 'text-foreground' : 'text-muted-foreground'} hover:text-foreground`}
              >
                <span className={`z-10 grid size-7 place-items-center rounded-full border-2 text-xs ${DOT[state]}`}>
                  {step.state === 'done' ? <Check className="size-4" data-testid={`step-check-${step.key}`} /> : index + 1}
                </span>
                <span>{label(step.key)}</span>
                {stateWord(step) ? <span className="sr-only">, {stateWord(step)}</span> : null}
                {step.key === 'import' ? <span className="text-[10px] text-muted-foreground">{t('onboarding.steps_nav.optional')}</span> : null}
              </button>
            </li>
          )
        })}
      </ol>
    </nav>
  )
}
