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
  // categories and accounts are done, so the rest of the guide may be wrapped up from this step
  coreStepsDone?: boolean
  // skips every open step, completes onboarding and lands on `to` instead of the budget
  onCompleteSetup?: (to: string) => Promise<void>
}

const STEP_BODIES: Record<OnboardingStepKey, ComponentType<StepBodyProps>> = {
  import: ImportStep,
  categories: CategoriesStepBody,
  accounts: AccountsStepBody,
  plan: PlanStep,
  home_screen: InstallStep,
  apple_wallet: AppleWalletStep,
}

const DONE_STEP_PAGES: Partial<Record<OnboardingStepKey, string>> = {
  categories: RouterPage.SETTINGS_CATEGORIES,
  accounts: RouterPage.SETTINGS_ACCOUNTS,
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
  // where a completion lands, kept so the Finish retry after a failure goes there too
  const landing = useRef<string>(RouterPage.BUDGET)

  const finish = useCallback(async () => {
    if (completing.current) return
    completing.current = true
    try {
      await complete.mutateAsync()
      if (userId) clearDraft(userId)
      navigate(landing.current)
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

  // a done step is read-only, so a selection that became done falls back to the first open step
  const active = steps.find((s) => s.key === selected && s.state !== 'done')?.key ?? current ?? steps[0].key

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

  const completeSetup = async (to?: string) => {
    setSkippingAll(true)
    if (to) landing.current = to
    else trackEvent(METRICS.ONBOARDING_SKIP, { step: active })
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

  // a done step cannot be reopened, so a link to it opens the settings page holding its data
  const goTo = (key: OnboardingStepKey) => {
    const settings = DONE_STEP_PAGES[key]
    if (steps.find((s) => s.key === key)?.state === 'done' && settings) navigate(settings)
    else setSelected(key)
  }

  const Body = STEP_BODIES[active]
  const coreStepsDone = steps.filter((s) => s.key === 'categories' || s.key === 'accounts').every((s) => s.state === 'done')

  return (
    <div className="mx-auto flex h-full w-full max-w-3xl flex-col gap-6 overflow-y-auto p-4 scrollbar-slim lg:px-8">
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
        <Button type="button" variant="ghost" size="sm" className="text-muted-foreground" disabled={skippingAll} onClick={() => void completeSetup()}>
          {t('onboarding.steps_nav.complete_setup')}
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
      {allResolved ? null : <Body key={active} onDone={answer(markDone)} onSkip={answer(skip)} onGoTo={goTo} coreStepsDone={coreStepsDone} onCompleteSetup={completeSetup} />}
    </div>
  )
}
