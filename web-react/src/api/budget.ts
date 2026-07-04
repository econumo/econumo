import { api, apiUrl } from './client'
import type { Id } from './types'
import type { BudgetMetaDto } from './dto/budget'

interface Envelope<T> {
  data: T
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
