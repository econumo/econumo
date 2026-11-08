import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { RouterPage } from '@/app/router-pages'
import { apiErrorMessage } from '@/lib/apiError'
import { AccountsStep } from './AccountsStep'
import { CategoriesStep } from './CategoriesStep'
import { useDay0 } from './useDay0'

export function OnboardingPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { draft, ready, saveAccounts, finish, skip, pending, error } = useDay0()

  if (!ready) {
    return null
  }

  const step = draft?.step ?? 'accounts'
  const goBudget = () => navigate(RouterPage.BUDGET)
  // the failure stays in the hook's error state; the step stays mounted so a retry resumes
  const swallow = () => undefined
  const errorAlert =
    error != null ? (
      <p role="alert" className="text-center text-sm text-destructive">
        {apiErrorMessage(error)}
      </p>
    ) : undefined

  return (
    <div className="mx-auto flex h-full w-full max-w-xl flex-col gap-4 overflow-y-auto p-4">
      <p className="text-sm text-muted-foreground">{t('onboarding.step_of', { current: step === 'accounts' ? 1 : 2, total: 2 })}</p>
      {step === 'accounts' ? (
        <AccountsStep
          initialRows={draft?.rows}
          initialExtraCurrencyIds={draft?.extraCurrencyIds}
          createdKeys={new Set(Object.keys(draft?.created ?? {}))}
          pending={pending}
          error={errorAlert}
          onContinue={(rows, extra, main) => void saveAccounts(rows, extra, main).catch(swallow)}
          onSkip={() => void skip().then(goBudget, swallow)}
        />
      ) : (
        <CategoriesStep
          initialRows={draft?.categoryRows.length ? draft.categoryRows : undefined}
          createdKeys={new Set(Object.keys(draft?.createdCategories ?? {}))}
          pending={pending}
          error={errorAlert}
          onContinue={(rows) => void finish(rows).then(goBudget, swallow)}
          onSkip={() => void skip().then(goBudget, swallow)}
        />
      )}
    </div>
  )
}
