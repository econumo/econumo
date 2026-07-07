import { api, apiUrl } from './client'
import type { Id } from './types'
import type { PayeeDto } from './dto/payee'

interface Envelope<T> {
  data: T
}

export async function getPayeeList(): Promise<PayeeDto[]> {
  const response = await api.get<Envelope<{ items: PayeeDto[] }>>(apiUrl('/api/v1/payee/get-payee-list'))
  return response.data.data.items
}

export async function createPayee(form: { id: Id; name: string; accountId?: Id }): Promise<PayeeDto> {
  const response = await api.post<Envelope<{ item: PayeeDto }>>(apiUrl('/api/v1/payee/create-payee'), form)
  return response.data.data.item
}

export async function updatePayee(form: { id: Id; name: string }): Promise<void> {
  await api.post(apiUrl('/api/v1/payee/update-payee'), form)
}

export async function archivePayee(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/payee/archive-payee'), { id })
}

export async function unarchivePayee(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/payee/unarchive-payee'), { id })
}

export async function deletePayee(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/payee/delete-payee'), { id })
}

export async function orderPayeeList(changes: { id: Id; position: number }[]): Promise<PayeeDto[]> {
  const response = await api.post<Envelope<{ items: PayeeDto[] }>>(apiUrl('/api/v1/payee/order-payee-list'), { changes })
  return response.data.data.items
}
