import { api, apiUrl } from './client'
import type { Id } from './types'
import type { CurrencyDto, CurrencyRateDto } from './dto/currency'

interface Envelope<T> {
  data: T
}

export async function getCurrencyList(): Promise<CurrencyDto[]> {
  const response = await api.get<Envelope<{ items: CurrencyDto[] }>>(apiUrl('/api/v1/currency/get-currency-list'))
  return response.data.data.items
}

export async function getCurrencyRateList(): Promise<CurrencyRateDto[]> {
  const response = await api.get<Envelope<{ items: CurrencyRateDto[] }>>(apiUrl('/api/v1/currency/get-currency-rate-list'))
  return response.data.data.items.map((r) => ({ ...r, rate: Number(r.rate) }))
}

export async function createCurrency(form: {
  id: Id
  code: string
  name: string
  symbol?: string
  fractionDigits?: number
  rate?: string
}): Promise<CurrencyDto> {
  const response = await api.post<Envelope<{ item: CurrencyDto }>>(apiUrl('/api/v1/currency/create-currency'), form)
  return response.data.data.item
}

export async function updateCurrency(form: { id: Id; name: string; symbol: string; fractionDigits: number }): Promise<void> {
  await api.post(apiUrl('/api/v1/currency/update-currency'), form)
}

export async function setCurrencyRate(form: { currencyId: Id; rate: string; date?: string }): Promise<void> {
  await api.post(apiUrl('/api/v1/currency/set-currency-rate'), form)
}

export async function archiveCurrency(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/currency/archive-currency'), { id })
}

export async function unarchiveCurrency(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/currency/unarchive-currency'), { id })
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
