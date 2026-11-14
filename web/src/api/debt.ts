import { api, apiUrl } from './client'
import type { Id } from './types'
import type {
  AddDebtRateChangeForm,
  AddDebtRuleDto,
  CreateDebtDto,
  DebtDetailsDto,
  DebtDto,
  DebtForecastDto,
  DebtRuleOptionsDto,
  DeleteDebtForm,
  DeleteDebtRateChangeForm,
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

/** follows the plans of the debt's own budget */
export async function getDebtForecast(id: Id): Promise<DebtForecastDto> {
  const response = await api.get<Envelope<DebtForecastDto>>(apiUrl(`/api/v1/debt/get-debt-forecast?id=${encodeURIComponent(id)}`))
  return response.data.data
}

export async function getDebtRuleOptions(budgetId: Id): Promise<DebtRuleOptionsDto> {
  const response = await api.get<Envelope<DebtRuleOptionsDto>>(apiUrl(`/api/v1/debt/get-debt-rule-options?budgetId=${encodeURIComponent(budgetId)}`))
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

export async function addDebtRule(form: AddDebtRuleDto): Promise<DebtDto> {
  const response = await api.post<Envelope<DebtDto>>(apiUrl('/api/v1/debt/add-debt-rule'), form)
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

export async function addDebtRateChange(form: AddDebtRateChangeForm): Promise<DebtDto> {
  const response = await api.post<Envelope<DebtDto>>(apiUrl('/api/v1/debt/add-debt-rate-change'), form)
  return response.data.data
}

export async function deleteDebtRateChange(form: DeleteDebtRateChangeForm): Promise<DebtDto> {
  const response = await api.post<Envelope<DebtDto>>(apiUrl('/api/v1/debt/delete-debt-rate-change'), form)
  return response.data.data
}
