import { api, apiUrl } from './client'
import type { Id } from './types'
import type { CurrencyListItemDto, CurrencyRateDto } from './dto/currency'

interface Envelope<T> {
  data: T
}

export async function getCurrencyList(): Promise<CurrencyListItemDto[]> {
  const response = await api.get<Envelope<{ items: CurrencyListItemDto[] }>>(apiUrl('/api/v1/currency/get-currency-list'))
  return response.data.data.items
}

export async function getCurrencyRateList(): Promise<CurrencyRateDto[]> {
  const response = await api.get<Envelope<{ items: CurrencyRateDto[] }>>(apiUrl('/api/v1/currency/get-currency-rate-list'))
  return response.data.data.items
}

export async function createCurrency(form: {
  id: Id
  code: string
  name: string
  symbol?: string
  fractionDigits?: number
  rate: string
}): Promise<CurrencyListItemDto> {
  const response = await api.post<Envelope<{ item: CurrencyListItemDto }>>(apiUrl('/api/v1/currency/create-currency'), form)
  return response.data.data.item
}

export async function updateCurrency(form: { id: Id; name: string; symbol: string; fractionDigits: number; rate: string }): Promise<void> {
  await api.post(apiUrl('/api/v1/currency/update-currency'), form)
}

export async function deleteCurrency(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/currency/delete-currency'), { id })
}

export async function hideCurrency(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/currency/hide-currency'), { id })
}

export async function showCurrency(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/currency/show-currency'), { id })
}

export async function hideAllCurrencies(): Promise<void> {
  await api.post(apiUrl('/api/v1/currency/hide-all-currencies'), {})
}

export async function showAllCurrencies(): Promise<void> {
  await api.post(apiUrl('/api/v1/currency/show-all-currencies'), {})
}
