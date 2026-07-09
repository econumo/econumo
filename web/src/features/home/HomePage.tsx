import { BudgetPage } from '@/features/budgets/BudgetPage'
import { OnboardingPage } from '@/features/onboarding/OnboardingPage'
import { isOnboardingCompleted, useUserData } from '@/features/user/queries'

// Vue's Home: Budget for onboarded users, Onboarding otherwise.
export function HomePage() {
  const { data: user } = useUserData()
  if (!user) {
    return <div data-testid="home-placeholder" />
  }
  return isOnboardingCompleted(user) ? <BudgetPage /> : <OnboardingPage />
}
