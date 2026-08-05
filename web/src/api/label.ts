import { api, apiUrl } from './client'
import type { Id } from './types'
import type { LabelDto } from './dto/label'

interface Envelope<T> {
  data: T
}

export async function getLabelList(): Promise<LabelDto[]> {
  const response = await api.get<Envelope<{ items: LabelDto[] }>>(apiUrl('/api/v1/label/get-label-list'))
  return response.data.data.items
}

export async function createLabel(form: { id: Id; name: string; accountId?: Id }): Promise<LabelDto> {
  const response = await api.post<Envelope<{ item: LabelDto }>>(apiUrl('/api/v1/label/create-label'), form)
  return response.data.data.item
}

export async function updateLabel(form: { id: Id; name: string }): Promise<LabelDto> {
  const response = await api.post<Envelope<{ item: LabelDto }>>(apiUrl('/api/v1/label/update-label'), form)
  return response.data.data.item
}

export async function archiveLabel(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/label/archive-label'), { id })
}

export async function unarchiveLabel(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/label/unarchive-label'), { id })
}

export async function deleteLabel(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/label/delete-label'), { id })
}

export async function moveLabel(id: Id, afterId: Id | null): Promise<LabelDto[]> {
  const response = await api.post<Envelope<{ items: LabelDto[] }>>(apiUrl('/api/v1/label/move-label'), { id, afterId })
  return response.data.data.items
}

export async function sortLabelList(ids: Id[]): Promise<LabelDto[]> {
  const response = await api.post<Envelope<{ items: LabelDto[] }>>(apiUrl('/api/v1/label/sort-label-list'), { ids })
  return response.data.data.items
}
