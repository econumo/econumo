import { api, apiUrl } from './client'
import type { Id } from './types'
import type { TagDto } from './dto/tag'

interface Envelope<T> {
  data: T
}

export async function getTagList(): Promise<TagDto[]> {
  const response = await api.get<Envelope<{ items: TagDto[] }>>(apiUrl('/api/v1/tag/get-tag-list'))
  return response.data.data.items
}

export async function createTag(form: { id: Id; name: string; accountId?: Id }): Promise<TagDto> {
  const response = await api.post<Envelope<{ item: TagDto }>>(apiUrl('/api/v1/tag/create-tag'), form)
  return response.data.data.item
}

export async function updateTag(form: { id: Id; name: string }): Promise<TagDto> {
  const response = await api.post<Envelope<{ item: TagDto }>>(apiUrl('/api/v1/tag/update-tag'), form)
  return response.data.data.item
}

export async function archiveTag(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/tag/archive-tag'), { id })
}

export async function unarchiveTag(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/tag/unarchive-tag'), { id })
}

export async function deleteTag(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/tag/delete-tag'), { id })
}

export async function orderTagList(changes: { id: Id; position: number }[]): Promise<TagDto[]> {
  const response = await api.post<Envelope<{ items: TagDto[] }>>(apiUrl('/api/v1/tag/order-tag-list'), { changes })
  return response.data.data.items
}
