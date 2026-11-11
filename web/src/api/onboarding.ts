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
  // the row's default icon, nobody chose it: with an AI provider the server may pick a better one
  autoIcon?: boolean
}

export interface SaveCategoriesForm {
  id: Id
  categories: OnboardingCategoryRowForm[]
  // existing category ids and row keys, each type in display order; absent leaves the order alone
  order?: string[]
  // own category ids to archive / bring back; absent leaves them alone
  archive?: Id[]
  restore?: Id[]
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
  // as on a category row
  autoIcon?: boolean
}

export interface SaveAccountsForm {
  id: Id
  accounts: OnboardingAccountRowForm[]
  currencyIds: Id[]
  mainCurrencyId: Id
  // own account ids: hide moves them into the hidden Archive folder and out of the budget, unhide brings them back
  hide?: Id[]
  unhide?: Id[]
  // own existing accounts moved to another currency (a CSV import creates them in the instance base currency)
  currencies?: AccountCurrencyChange[]
  // own existing accounts' new balances: the server books the difference as a Balance adjustment
  balances?: AccountBalanceChange[]
}

export interface AccountBalanceChange {
  accountId: Id
  balance: string
}

export interface AccountCurrencyChange {
  accountId: Id
  currencyId: Id
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

export interface FinishImportResult {
  user: CurrentUserDto
  // the budget the call created, or the user's existing default one; '' without any account
  budgetId: Id | ''
}

// Marks the import step done and creates the budget when the user has none. When the
// server has an AI provider it sets icons and savings flags and creates debts from loan
// categories in the background afterwards.
export async function finishOnboardingImport(id: Id): Promise<FinishImportResult> {
  const response = await api.post<Envelope<FinishImportResult>>(apiUrl('/api/v1/onboarding/finish-import'), { id })
  return response.data.data
}

export interface PlanSuggestions {
  budgetId: Id | ''
  // a decimal string per category with spending (or income) in at least two of the last three months
  items: { categoryId: Id; amount: string }[]
}

export async function getOnboardingPlanSuggestions(budgetId: Id): Promise<PlanSuggestions> {
  const response = await api.get<Envelope<PlanSuggestions>>(apiUrl('/api/v1/onboarding/get-plan-suggestions'), { params: { budgetId } })
  return response.data.data
}
