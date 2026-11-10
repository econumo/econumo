import { api, apiUrl } from './client'
import type { CurrentUserDto } from './dto/user'
import type { Id } from './types'

interface Envelope<T> {
  data: T
}

// Each Day 0 step is one server transaction: it creates the rows, marks the
// step done in the onboarding option and answers with the updated user. `id`
// is the operation id; a replay of it, or a call for a step already done,
// writes nothing and answers with the current state.

export interface OnboardingCategoryRowForm {
  key: string
  name: string
  type: 'expense' | 'income'
  icon: string
}

export interface SaveCategoriesForm {
  id: Id
  categories: OnboardingCategoryRowForm[]
  // existing category ids and row keys, each type in display order; absent leaves the order alone
  order?: string[]
}

export interface SaveCategoriesResult {
  user: CurrentUserDto
  categories: Record<string, Id>
  budgetId: Id | ''
}

export async function saveOnboardingCategories(form: SaveCategoriesForm): Promise<SaveCategoriesResult> {
  const response = await api.post<Envelope<SaveCategoriesResult>>(apiUrl('/api/v1/onboarding/save-categories'), form)
  return response.data.data
}

export interface OnboardingAccountRowForm {
  key: string
  name: string
  balance: string
  currencyId: Id
  icon: string
}

export interface SaveAccountsForm {
  id: Id
  accounts: OnboardingAccountRowForm[]
  currencyIds: Id[]
  mainCurrencyId: Id
}

export interface SaveAccountsResult {
  user: CurrentUserDto
  accounts: Record<string, Id>
  budgetId: Id | ''
}

// When the server has an AI provider it flags the new savings accounts in the background afterwards.
export async function saveOnboardingAccounts(form: SaveAccountsForm): Promise<SaveAccountsResult> {
  const response = await api.post<Envelope<SaveAccountsResult>>(apiUrl('/api/v1/onboarding/save-accounts'), form)
  return response.data.data
}

export interface SavePlanForm {
  id: Id
  budgetId?: Id
  // limits of the budget's current period; "0" is a real zero plan
  amounts: { categoryId: Id; amount: string }[]
}

export interface SavePlanResult {
  user: CurrentUserDto
  budgetId: Id
}

export async function saveOnboardingPlan(form: SavePlanForm): Promise<SavePlanResult> {
  const response = await api.post<Envelope<SavePlanResult>>(apiUrl('/api/v1/onboarding/save-plan'), form)
  return response.data.data
}
