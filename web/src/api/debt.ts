import { api, apiUrl } from './client'
import type { Id } from './types'
import type {
  CreateDebtDto,
  DebtDetailsDto,
  DebtDto,
  DebtForecastDto,
  DeleteDebtForm,
  DeleteDebtReconciliationForm,
  ReconcileDebtForm,
  UpdateDebtDto,
} from './dto/debt'

interface Envelope<T> {
  data: T
}

export async function getDebtList(): Promise<DebtDto[]> {
  const response = await api.get<Envelope<{ items: DebtDto[] }>>(apiUrl('/api/v1/debt/get-debt-list'))
  return response.data.data.items
}

export async function getDebt(id: Id): Promise<DebtDetailsDto> {
  const response = await api.get<Envelope<DebtDetailsDto>>(apiUrl(`/api/v1/debt/get-debt?id=${encodeURIComponent(id)}`))
  return response.data.data
}

/** without a budget the server follows the user's active budget, if it plans the debt */
export async function getDebtForecast(id: Id, budgetId: Id | null): Promise<DebtForecastDto> {
  const params = new URLSearchParams({ id })
  if (budgetId) {
    params.set('budgetId', budgetId)
  }
  const response = await api.get<Envelope<DebtForecastDto>>(apiUrl(`/api/v1/debt/get-debt-forecast?${params}`))
  return response.data.data
}

export async function createDebt(form: CreateDebtDto): Promise<DebtDto> {
  const response = await api.post<Envelope<DebtDto>>(apiUrl('/api/v1/debt/create-debt'), form)
  return response.data.data
}

export async function updateDebt(form: UpdateDebtDto): Promise<DebtDto> {
  const response = await api.post<Envelope<DebtDto>>(apiUrl('/api/v1/debt/update-debt'), form)
  return response.data.data
}

export async function deleteDebt(form: DeleteDebtForm): Promise<void> {
  await api.post(apiUrl('/api/v1/debt/delete-debt'), form)
}

export async function markDebtPaidOff(id: Id): Promise<DebtDto> {
  const response = await api.post<Envelope<DebtDto>>(apiUrl('/api/v1/debt/mark-debt-paid-off'), { id })
  return response.data.data
}

export async function reconcileDebt(form: ReconcileDebtForm): Promise<DebtDto> {
  const response = await api.post<Envelope<DebtDto>>(apiUrl('/api/v1/debt/reconcile-debt'), form)
  return response.data.data
}

export async function deleteDebtReconciliation(form: DeleteDebtReconciliationForm): Promise<DebtDto> {
  const response = await api.post<Envelope<DebtDto>>(apiUrl('/api/v1/debt/delete-debt-reconciliation'), form)
  return response.data.data
}
