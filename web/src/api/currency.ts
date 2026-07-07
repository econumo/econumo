import { api, apiUrl } from './client'
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
