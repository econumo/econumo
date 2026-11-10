import { api, apiUrl } from './client'
import type { Id } from './types'
import type { BudgetMetaDto } from './dto/budget'

interface Envelope<T> {
  data: T
}

export interface OnboardingBudgetForm {
  /** the budget id, also the create's idempotency key */
  id: Id
  name: string
  startDate: string
  currencyId: Id
  accountIds: Id[]
}

// Creates the Day 0 budget. When the server has an AI provider it flags the
// savings accounts in the background afterwards; the client never waits for it.
export async function createOnboardingBudget(form: OnboardingBudgetForm): Promise<BudgetMetaDto> {
  const response = await api.post<Envelope<{ item: { meta: BudgetMetaDto } }>>(apiUrl('/api/v1/onboarding/create-budget'), form)
  return response.data.data.item.meta
}

// Adds accounts to the budget as everyday members (same background savings
// classification as createOnboardingBudget).
export async function addOnboardingBudgetAccounts(budgetId: Id, accountIds: Id[]): Promise<BudgetMetaDto> {
  const response = await api.post<Envelope<{ item: BudgetMetaDto }>>(apiUrl('/api/v1/onboarding/add-budget-accounts'), { budgetId, accountIds })
  return response.data.data.item
}
