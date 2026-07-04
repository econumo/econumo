import { BudgetPage } from '@/features/budgets/BudgetPage'
import { isOnboardingCompleted, useUserData } from '@/features/user/queries'

// Vue's Home: Budget for onboarded users, Onboarding otherwise.
// The Onboarding page arrives in Plan 5; until then the placeholder stays.
export function HomePage() {
  const { data: user } = useUserData()
  if (user && isOnboardingCompleted(user)) {
    return <BudgetPage />
  }
  return <div data-testid="home-placeholder" />
}
