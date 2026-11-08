import { useCallback, useEffect, useRef, useState, type ComponentType } from 'react'
import { useTranslation } from 'react-i18next'
import { Menu } from 'lucide-react'
import { Link, useNavigate } from 'react-router'
import { RouterPage } from '@/app/router-pages'
import { Button } from '@/components/ui/button'
import { useCompleteOnboarding, useUserData } from '@/features/user/queries'
import { useIsCompact } from '@/hooks/useIsCompact'
import { apiErrorMessage } from '@/lib/apiError'
import { METRICS, trackEvent } from '@/lib/metrics'
import { AccountsStepBody } from './AccountsStep'
import { AppleWalletStep } from './AppleWalletStep'
import { CategoriesStepBody } from './CategoriesStep'
import { clearDraft } from './draft'
import { ImportStep } from './ImportStep'
import { InstallStep } from './InstallStep'
import { PlanStep } from './PlanStep'
import { Stepper } from './Stepper'
import { useOnboardingSteps, type OnboardingStepKey } from './useOnboardingSteps'

export interface StepBodyProps {
  // `also`: further steps the same action settled, written together with this one
  onDone: (also?: OnboardingStepKey[]) => Promise<void>
  onSkip: () => Promise<void>
  onGoTo?: (key: OnboardingStepKey) => void
}

const STEP_BODIES: Record<OnboardingStepKey, ComponentType<StepBodyProps>> = {
  import: ImportStep,
  categories: CategoriesStepBody,
  accounts: AccountsStepBody,
  plan: PlanStep,
  home_screen: InstallStep,
  apple_wallet: AppleWalletStep,
}

export function OnboardingPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isCompact = useIsCompact()
  const { steps, current, allResolved, markDone, markManyDone, skip, ready } = useOnboardingSteps()
  const complete = useCompleteOnboarding()
  const { data: user } = useUserData()
  const userId = user?.id
  const [selected, setSelected] = useState<OnboardingStepKey | null>(null)
  const [skippingAll, setSkippingAll] = useState(false)
  const completing = useRef(false)
  const autoCompleted = useRef(false)

  const finish = useCallback(async () => {
    if (completing.current) return
    completing.current = true
    try {
      await complete.mutateAsync()
      if (userId) clearDraft(userId)
      navigate(RouterPage.BUDGET)
    } finally {
      completing.current = false
    }
  }, [complete, navigate, userId])

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
  const answer = (act: (key: OnboardingStepKey) => Promise<void>) => async (also: OnboardingStepKey[] = []) => {
    if (also.length > 0) await markManyDone([active, ...also])
    else await act(active)
    const open = steps.filter((s) => s.key !== active && !also.includes(s.key) && s.state === 'open')
    if (open.length === 0) {
      autoCompleted.current = true
      await finish().catch(() => undefined)
      return
    }
    const after = open.find((s) => steps.indexOf(s) > steps.findIndex((x) => x.key === active))
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
      {/* the page title is spoken; the stepper's own line shows it on a phone */}
      <h1 className="sr-only">{t('onboarding.steps_nav.title')}</h1>
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
      {allResolved ? null : <Body key={active} onDone={answer(markDone)} onSkip={answer(skip)} onGoTo={setSelected} />}
    </div>
  )
}
