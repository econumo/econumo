import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { v7 as uuidv7 } from 'uuid'
import * as categoryApi from '@/api/category'
import * as payeeApi from '@/api/payee'
import * as tagApi from '@/api/tag'
import type { CategoryDto, CategoryType } from '@/api/dto/category'
import type { PayeeDto } from '@/api/dto/payee'
import type { TagDto } from '@/api/dto/tag'
import type { Id } from '@/api/types'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'

const byPosition = <T extends { position: number }>(items: T[]) => [...items].sort((a, b) => a.position - b.position)

export function useCategories() {
  return useQuery({ queryKey: queryKeys.categories, queryFn: categoryApi.getCategoryList, staleTime: TEN_MINUTES, select: byPosition })
}

export function usePayees() {
  return useQuery({ queryKey: queryKeys.payees, queryFn: payeeApi.getPayeeList, staleTime: TEN_MINUTES, select: byPosition })
}

export function useTags() {
  return useQuery({ queryKey: queryKeys.tags, queryFn: tagApi.getTagList, staleTime: TEN_MINUTES, select: byPosition })
}

// The Vue stores dedupe by lowercased name among the owner's items before creating.
function findByName<T extends { name: string; ownerUserId: Id }>(items: T[] | undefined, name: string, ownerUserId?: Id): T | undefined {
  const target = name.toLowerCase()
  return (items ?? []).find((i) => i.name.toLowerCase() === target && (!ownerUserId || i.ownerUserId === ownerUserId))
}

export function useCreateCategory() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (form: { name: string; type: CategoryType; accountId?: Id; ownerUserId?: Id }) => {
      const existing = findByName(queryClient.getQueryData<CategoryDto[]>(queryKeys.categories), form.name, form.ownerUserId)
      if (existing && existing.type === form.type) {
        return existing
      }
      return categoryApi.createCategory({ id: uuidv7(), name: form.name, type: form.type, accountId: form.accountId })
    },
    onSuccess: (item) => {
      queryClient.setQueryData<CategoryDto[]>(queryKeys.categories, (prev) => {
        const items = prev ?? []
        return items.some((c) => c.id === item.id) ? items : [...items, item]
      })
    },
  })
}

export function useCreatePayee() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (form: { name: string; accountId?: Id; ownerUserId?: Id }) => {
      const existing = findByName(queryClient.getQueryData<PayeeDto[]>(queryKeys.payees), form.name, form.ownerUserId)
      if (existing) {
        return existing
      }
      return payeeApi.createPayee({ id: uuidv7(), name: form.name, accountId: form.accountId })
    },
    onSuccess: (item) => {
      queryClient.setQueryData<PayeeDto[]>(queryKeys.payees, (prev) => {
        const items = prev ?? []
        return items.some((p) => p.id === item.id) ? items : [...items, item]
      })
    },
  })
}

export function useCreateTag() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (form: { name: string; accountId?: Id; ownerUserId?: Id }) => {
      const existing = findByName(queryClient.getQueryData<TagDto[]>(queryKeys.tags), form.name, form.ownerUserId)
      if (existing) {
        return existing
      }
      return tagApi.createTag({ id: uuidv7(), name: form.name, accountId: form.accountId })
    },
    onSuccess: (item) => {
      queryClient.setQueryData<TagDto[]>(queryKeys.tags, (prev) => {
        const items = prev ?? []
        return items.some((t) => t.id === item.id) ? items : [...items, item]
      })
    },
  })
}
