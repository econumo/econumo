import { api, apiUrl } from './client'
import type { Id } from './types'
import type { CreateRecurringDto, PostRecurringPayload, PostRecurringResult, RecurringDto } from './dto/recurring'
import { coerceAccount, coerceTransaction } from './account'

interface Envelope<T> {
  data: T
}

function coerceRecurring(raw: RecurringDto): RecurringDto {
  return { ...raw, amount: Number(raw.amount) }
}

export async function getRecurringList(): Promise<RecurringDto[]> {
  const response = await api.get<Envelope<{ items: RecurringDto[] }>>(apiUrl('/api/v1/recurring/get-recurring-transaction-list'))
  return response.data.data.items.map(coerceRecurring)
}

export async function createRecurring(form: CreateRecurringDto): Promise<RecurringDto> {
  const response = await api.post<Envelope<{ item: RecurringDto }>>(apiUrl('/api/v1/recurring/create-recurring-transaction'), form)
  return coerceRecurring(response.data.data.item)
}

export async function updateRecurring(form: CreateRecurringDto): Promise<RecurringDto> {
  const response = await api.post<Envelope<{ item: RecurringDto }>>(apiUrl('/api/v1/recurring/update-recurring-transaction'), form)
  return coerceRecurring(response.data.data.item)
}

export async function deleteRecurring(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/recurring/delete-recurring-transaction'), { id })
}

export async function skipRecurring(id: Id): Promise<RecurringDto> {
  const response = await api.post<Envelope<{ item: RecurringDto }>>(apiUrl('/api/v1/recurring/skip-recurring-transaction'), { id })
  return coerceRecurring(response.data.data.item)
}

export async function postRecurring(payload: PostRecurringPayload): Promise<PostRecurringResult> {
  const response = await api.post<Envelope<PostRecurringResult>>(apiUrl('/api/v1/recurring/post-recurring-transaction'), payload)
  const { item, accounts, nextPaymentAt } = response.data.data
  return { item: coerceTransaction(item), accounts: accounts.map(coerceAccount), nextPaymentAt }
}
