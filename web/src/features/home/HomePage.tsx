import { BudgetPage } from '@/features/budgets/BudgetPage'
import { useBudgetPeriodStore } from '@/features/budgets/budgetStore'
import { OnboardingPage } from '@/features/onboarding/OnboardingPage'
import { isOnboardingCompleted, useUserData } from '@/features/user/queries'

// Budget or Plan (whichever this device opened last) for onboarded users, Onboarding otherwise.
// Rendering a fixed mode here would overwrite the remembered view on every app start.
export function HomePage() {
  const { data: user } = useUserData()
  const lastMode = useBudgetPeriodStore((s) => s.lastMode)
  if (!user) {
    return <div data-testid="home-placeholder" />
  }
  return isOnboardingCompleted(user) ? <BudgetPage key={lastMode} mode={lastMode} /> : <OnboardingPage />
}
