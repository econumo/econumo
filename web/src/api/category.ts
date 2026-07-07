import { api, apiUrl } from './client'
import type { Id } from './types'
import type { CategoryDto, CategoryType } from './dto/category'

interface Envelope<T> {
  data: T
}

export async function getCategoryList(): Promise<CategoryDto[]> {
  const response = await api.get<Envelope<{ items: CategoryDto[] }>>(apiUrl('/api/v1/category/get-category-list'))
  return response.data.data.items
}

export async function createCategory(form: { id: Id; name: string; type: CategoryType; accountId?: Id; icon?: string }): Promise<CategoryDto> {
  const response = await api.post<Envelope<{ item: CategoryDto }>>(apiUrl('/api/v1/category/create-category'), form)
  return response.data.data.item
}

export async function updateCategory(form: { id: Id; name: string; icon: string }): Promise<void> {
  await api.post(apiUrl('/api/v1/category/update-category'), form)
}

export async function archiveCategory(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/category/archive-category'), { id })
}

export async function unarchiveCategory(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/category/unarchive-category'), { id })
}

export async function deleteCategory(id: Id, replaceId?: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/category/delete-category'), replaceId ? { id, replaceId, mode: 'replace' } : { id, mode: 'delete' })
}

export async function orderCategoryList(changes: { id: Id; position: number }[]): Promise<CategoryDto[]> {
  const response = await api.post<Envelope<{ items: CategoryDto[] }>>(apiUrl('/api/v1/category/order-category-list'), { changes })
  return response.data.data.items
}
