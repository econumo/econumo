import { useCallback, useEffect, useRef, useState, type ComponentType } from 'react'
import { useTranslation } from 'react-i18next'
import { Menu } from 'lucide-react'
import { Link, useNavigate } from 'react-router'
import { RouterPage } from '@/app/router-pages'
import { Button } from '@/components/ui/button'
import { useCompleteOnboarding } from '@/features/user/queries'
import { useIsCompact } from '@/hooks/useIsCompact'
import { apiErrorMessage } from '@/lib/apiError'
import { METRICS, trackEvent } from '@/lib/metrics'
import { Stepper } from './Stepper'
import { useOnboardingSteps, type OnboardingStepKey } from './useOnboardingSteps'

export interface StepBodyProps {
  onDone: () => Promise<void>
  onSkip: () => Promise<void>
}

function placeholderBody(key: OnboardingStepKey): ComponentType<StepBodyProps> {
  return function PlaceholderStep({ onDone, onSkip }: StepBodyProps) {
    const { t } = useTranslation()
    const [pending, setPending] = useState(false)
    const run = (action: () => Promise<void>) => () => {
      setPending(true)
      void action().finally(() => setPending(false))
    }
    return (
      <section className="flex flex-col gap-2">
        <h2 className="text-2xl font-normal">{t(`onboarding.steps_nav.${key}`)}</h2>
        <p className="text-sm text-muted-foreground">{t('onboarding.placeholder.text')}</p>
        <div className="mt-6 flex items-center justify-between">
          <Button type="button" variant="ghost" disabled={pending} onClick={run(onSkip)}>
            {t('onboarding.placeholder.skip')}
          </Button>
          <Button type="button" disabled={pending} onClick={run(onDone)}>
            {t('onboarding.placeholder.continue')}
          </Button>
        </div>
      </section>
    )
  }
}

const STEP_BODIES: Record<OnboardingStepKey, ComponentType<StepBodyProps>> = {
  import: placeholderBody('import'),
  categories: placeholderBody('categories'),
  accounts: placeholderBody('accounts'),
  plan: placeholderBody('plan'),
  home_screen: placeholderBody('home_screen'),
  apple_wallet: placeholderBody('apple_wallet'),
}

export function OnboardingPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isCompact = useIsCompact()
  const { steps, current, allResolved, markDone, skip, ready } = useOnboardingSteps()
  const complete = useCompleteOnboarding()
  const [selected, setSelected] = useState<OnboardingStepKey | null>(null)
  const [skippingAll, setSkippingAll] = useState(false)
  const completing = useRef(false)
  const autoCompleted = useRef(false)

  const finish = useCallback(async () => {
    if (completing.current) return
    completing.current = true
    try {
      await complete.mutateAsync()
      navigate(RouterPage.BUDGET)
    } finally {
      completing.current = false
    }
  }, [complete, navigate])

  // Steps answered elsewhere (another device, the budget page) finish the
  // flow on arrival; only once, so a failure waits for the Finish button.
  useEffect(() => {
    if (ready && allResolved && !autoCompleted.current) {
      autoCompleted.current = true
      void finish().catch(() => undefined)
    }
  }, [ready, allResolved, finish])

  if (!ready || steps.length === 0) {
    return null
  }

  const active = steps.find((s) => s.key === selected)?.key ?? current ?? steps[0].key

  // The steps read here are the ones rendered, so the answered step still
  // counts as open in them and is excluded by key.
  const answer = (act: (key: OnboardingStepKey) => Promise<void>) => async () => {
    await act(active)
    const open = steps.filter((s) => s.key !== active && s.state === 'open')
    if (open.length === 0) {
      autoCompleted.current = true
      await finish().catch(() => undefined)
      return
    }
    const after = steps.slice(steps.findIndex((s) => s.key === active) + 1).find((s) => s.state === 'open')
    setSelected((after ?? open[0]).key)
  }

  const skipSetup = async () => {
    setSkippingAll(true)
    trackEvent(METRICS.ONBOARDING_SKIP, { step: active })
    try {
      await Promise.all(steps.filter((s) => s.state === 'open').map((s) => skip(s.key)))
      autoCompleted.current = true
      await finish()
    } catch {
      // the completion error stays in the mutation state and is shown below
    } finally {
      setSkippingAll(false)
    }
  }

  const Body = STEP_BODIES[active]

  return (
    <div className="mx-auto flex h-full w-full max-w-3xl flex-col gap-6 overflow-y-auto p-4 lg:px-8">
      <div className="flex items-center justify-between">
        {isCompact ? (
          <Link to="/?menu=1" aria-label={t('onboarding.steps_nav.menu')} className="-ml-2 grid size-10 place-items-center rounded-lg text-muted-foreground hover:bg-accent">
            <Menu className="size-5" />
          </Link>
        ) : (
          <span />
        )}
        <Button type="button" variant="ghost" size="sm" className="text-muted-foreground" disabled={skippingAll} onClick={() => void skipSetup()}>
          {t('onboarding.steps_nav.skip_setup')}
        </Button>
      </div>
      <Stepper steps={steps} current={active} onSelect={setSelected} />
      {complete.error != null ? (
        <div className="flex items-center justify-between gap-4">
          <p role="alert" className="text-sm text-destructive">
            {apiErrorMessage(complete.error)}
          </p>
          {allResolved ? (
            <Button type="button" disabled={complete.isPending} onClick={() => void finish().catch(() => undefined)}>
              {t('onboarding.steps_nav.finish')}
            </Button>
          ) : null}
        </div>
      ) : null}
      {allResolved ? null : <Body key={active} onDone={answer(markDone)} onSkip={answer(skip)} />}
    </div>
  )
}
