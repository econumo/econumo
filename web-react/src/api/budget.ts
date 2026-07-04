import { api, apiUrl } from './client'
import type { Id } from './types'
import type { BudgetBalanceDto, BudgetDto, BudgetElementDto, BudgetFolderDto, BudgetMetaDto } from './dto/budget'
import type { TransactionDto } from './dto/transaction'
import { coerceTransaction } from './account'

interface Envelope<T> {
  data: T
}

const num = (v: unknown): number => Number(v)
const numOrNull = (v: unknown): number | null => (v === null || v === undefined ? null : Number(v))

// The exact Vue coercion list: parent spent/budgetSpent/budgeted/available,
// child spent/budgetSpent, balances (null-preserving), rate.
function coerceBudget(raw: BudgetDto): BudgetDto {
  return {
    ...raw,
    balances: raw.balances.map((b): BudgetBalanceDto => ({
      ...b,
      startBalance: numOrNull(b.startBalance),
      endBalance: numOrNull(b.endBalance),
      income: numOrNull(b.income),
      expenses: numOrNull(b.expenses),
      exchanges: numOrNull(b.exchanges),
      holdings: numOrNull(b.holdings),
    })),
    currencyRates: raw.currencyRates.map((r) => ({ ...r, rate: num(r.rate) })),
    structure: {
      folders: raw.structure.folders,
      elements: raw.structure.elements.map((el): BudgetElementDto => ({
        ...el,
        spent: num(el.spent),
        budgetSpent: num(el.budgetSpent),
        budgeted: num(el.budgeted),
        available: num(el.available),
        children: el.children.map((child) => ({ ...child, spent: num(child.spent), budgetSpent: num(child.budgetSpent) })),
      })),
    },
  }
}

export interface CreateBudgetForm {
  /** unlike account/transaction create, this IS the entity id */
  id: Id
  name: string
  startDate: string | null
  currencyId: Id
  excludedAccounts: Id[]
}

export interface UpdateBudgetForm {
  id: Id
  name: string
  currencyId: Id
  excludedAccounts: Id[]
}

export async function getBudgetList(): Promise<BudgetMetaDto[]> {
  const response = await api.get<Envelope<{ items: BudgetMetaDto[] }>>(apiUrl('/api/v1/budget/get-budget-list'))
  return response.data.data.items
}

export async function createBudget(form: CreateBudgetForm): Promise<BudgetMetaDto> {
  const response = await api.post<Envelope<{ item: { meta: BudgetMetaDto } }>>(apiUrl('/api/v1/budget/create-budget'), form)
  return response.data.data.item.meta
}

export async function updateBudget(form: UpdateBudgetForm): Promise<BudgetMetaDto> {
  const response = await api.post<Envelope<{ item: BudgetMetaDto }>>(apiUrl('/api/v1/budget/update-budget'), form)
  return response.data.data.item
}

export async function deleteBudget(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/budget/delete-budget'), { id })
}

export async function getBudget(id: Id, date: string): Promise<BudgetDto> {
  const response = await api.get<Envelope<{ item: BudgetDto }>>(
    apiUrl(`/api/v1/budget/get-budget?id=${encodeURIComponent(id)}&date=${encodeURIComponent(date)}`),
  )
  return coerceBudget(response.data.data.item)
}

export interface SetLimitForm {
  budgetId: Id
  elementId: Id
  /** strict Y-m-d, first of month */
  period: string
  /** null clears the limit; "0" sets a real zero limit */
  amount: string | null
}

export async function setLimit(form: SetLimitForm): Promise<void> {
  await api.post(apiUrl('/api/v1/budget/set-limit'), form)
}

export interface EnvelopeForm {
  budgetId: Id
  id: Id
  name: string
  icon: string
  currencyId: Id
  folderId: Id | null
  categories: Id[]
}

export async function createEnvelope(form: EnvelopeForm): Promise<BudgetElementDto> {
  const response = await api.post<Envelope<{ item: BudgetElementDto }>>(apiUrl('/api/v1/budget/create-envelope'), form)
  return response.data.data.item
}

export async function updateEnvelope(form: Omit<EnvelopeForm, 'folderId'> & { isArchived: 0 | 1 }): Promise<BudgetElementDto> {
  const response = await api.post<Envelope<{ item: BudgetElementDto }>>(apiUrl('/api/v1/budget/update-envelope'), form)
  return response.data.data.item
}

export async function deleteEnvelope(budgetId: Id, id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/budget/delete-envelope'), { budgetId, id })
}

export async function createBudgetFolder(form: { budgetId: Id; id: Id; name: string }): Promise<BudgetFolderDto> {
  const response = await api.post<Envelope<{ item: BudgetFolderDto }>>(apiUrl('/api/v1/budget/create-folder'), form)
  return response.data.data.item
}

export async function updateBudgetFolder(form: { budgetId: Id; id: Id; name: string }): Promise<BudgetFolderDto> {
  const response = await api.post<Envelope<{ item: BudgetFolderDto }>>(apiUrl('/api/v1/budget/update-folder'), form)
  return response.data.data.item
}

export async function deleteBudgetFolder(budgetId: Id, id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/budget/delete-folder'), { budgetId, id })
}

export async function orderBudgetFolders(budgetId: Id, items: { id: Id; position: number }[]): Promise<void> {
  await api.post(apiUrl('/api/v1/budget/order-folder-list'), { budgetId, items })
}

export async function moveElements(budgetId: Id, items: { id: Id; folderId: Id | null; position: number }[]): Promise<void> {
  await api.post(apiUrl('/api/v1/budget/move-element-list'), { budgetId, items })
}

export async function changeElementCurrency(form: { budgetId: Id; elementId: Id; currencyId: Id }): Promise<void> {
  await api.post(apiUrl('/api/v1/budget/change-element-currency'), form)
}

export interface BudgetTransactionsParams {
  budgetId: Id
  periodStart: string
  categoryId?: Id
  tagId?: Id
  envelopeId?: Id
}

export async function getBudgetTransactions(params: BudgetTransactionsParams): Promise<TransactionDto[]> {
  const query = new URLSearchParams({ budgetId: params.budgetId, periodStart: params.periodStart })
  if (params.categoryId) query.set('categoryId', params.categoryId)
  if (params.tagId) query.set('tagId', params.tagId)
  if (params.envelopeId) query.set('envelopeId', params.envelopeId)
  const response = await api.get<Envelope<{ items: TransactionDto[] }>>(
    apiUrl(`/api/v1/budget/get-transaction-list?${query.toString()}`),
  )
  return response.data.data.items.map(coerceTransaction)
}

// Access + account inclusion functions for Plan 5 (no UI yet).
export async function grantAccess(form: { budgetId: Id; userId: Id; role: string }): Promise<BudgetMetaDto[]> {
  const response = await api.post<Envelope<{ items: BudgetMetaDto[] }>>(apiUrl('/api/v1/budget/grant-access'), form)
  return response.data.data.items
}

export async function revokeAccess(form: { budgetId: Id; userId: Id }): Promise<void> {
  await api.post(apiUrl('/api/v1/budget/revoke-access'), form)
}

export async function acceptAccess(budgetId: Id): Promise<BudgetMetaDto[]> {
  const response = await api.post<Envelope<{ items: BudgetMetaDto[] }>>(apiUrl('/api/v1/budget/accept-access'), { budgetId })
  return response.data.data.items
}

export async function declineAccess(budgetId: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/budget/decline-access'), { budgetId })
}

// NOTE the wire quirk: the budget id travels under "id", not budgetId.
export async function excludeAccount(budgetId: Id, accountId: Id): Promise<BudgetMetaDto> {
  const response = await api.post<Envelope<{ item: BudgetMetaDto }>>(apiUrl('/api/v1/budget/exclude-account'), { id: budgetId, accountId })
  return response.data.data.item
}

export async function includeAccount(budgetId: Id, accountId: Id): Promise<BudgetMetaDto> {
  const response = await api.post<Envelope<{ item: BudgetMetaDto }>>(apiUrl('/api/v1/budget/include-account'), { id: budgetId, accountId })
  return response.data.data.item
}
