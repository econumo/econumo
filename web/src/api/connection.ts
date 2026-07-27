import { api, apiUrl } from './client'
import type { Id } from './types'
import type { ConnectionDto, InviteDto } from './dto/connection'

interface Envelope<T> {
  data: T
}

export async function getConnectionList(): Promise<ConnectionDto[]> {
  const response = await api.get<Envelope<{ items: ConnectionDto[] }>>(apiUrl('/api/v1/connection/get-connection-list'))
  return response.data.data.items
}

export async function generateInvite(): Promise<InviteDto> {
  const response = await api.post<Envelope<{ item: InviteDto }>>(apiUrl('/api/v1/connection/generate-invite'), {})
  return response.data.data.item
}

export async function acceptInvite(code: string): Promise<ConnectionDto[]> {
  const response = await api.post<Envelope<{ items: ConnectionDto[] }>>(apiUrl('/api/v1/connection/accept-invite'), { code })
  return response.data.data.items
}

// the wire field is "id" (the connected user's id)
export async function deleteConnection(userId: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/connection/delete-connection'), { id: userId })
}
