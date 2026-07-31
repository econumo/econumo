import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { v7 as uuidv7 } from 'uuid'
import * as categoryApi from '@/api/category'
import * as payeeApi from '@/api/payee'
import * as tagApi from '@/api/tag'
import type { CategoryDto, CategoryType } from '@/api/dto/category'
import type { PayeeDto } from '@/api/dto/payee'
import type { TagDto } from '@/api/dto/tag'
import type { TransactionDto } from '@/api/dto/transaction'
import type { Id } from '@/api/types'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'

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
    mutationFn: async (form: { name: string; type: CategoryType; accountId?: Id; ownerUserId?: Id; icon?: string }) => {
      const existing = findByName(queryClient.getQueryData<CategoryDto[]>(queryKeys.categories), form.name, form.ownerUserId)
      if (existing && existing.type === form.type) {
        return existing
      }
      const item = await categoryApi.createCategory({ id: uuidv7(), name: form.name, type: form.type, accountId: form.accountId, icon: form.icon })
      // in mutationFn, not onSuccess: the dedupe path above is not a create
      trackEvent(METRICS.CATEGORY_CREATE)
      return item
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
      const item = await payeeApi.createPayee({ id: uuidv7(), name: form.name, accountId: form.accountId })
      trackEvent(METRICS.PAYEE_CREATE)
      return item
    },
    onSuccess: (item) => {
      queryClient.setQueryData<PayeeDto[]>(queryKeys.payees, (prev) => {
        const items = prev ?? []
        return items.some((p) => p.id === item.id) ? items : [...items, item]
      })
    },
  })
}

type EntityKind = 'categories' | 'payees' | 'tags'
type EntityDto = CategoryDto | PayeeDto | TagDto

function useEntityCacheOps(kind: EntityKind, touchesBudget: boolean) {
  const queryClient = useQueryClient()
  const key = queryKeys[kind]
  return {
    replaceItem: (id: Id, patch: Partial<EntityDto>) => {
      queryClient.setQueryData<EntityDto[]>(key, (prev) => (prev ?? []).map((i) => (i.id === id ? { ...i, ...patch } : i)))
      if (touchesBudget) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      }
    },
    setArchived: (id: Id, isArchived: 0 | 1) => {
      queryClient.setQueryData<EntityDto[]>(key, (prev) => (prev ?? []).map((i) => (i.id === id ? { ...i, isArchived } : i)))
      if (touchesBudget) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      }
    },
    remove: (id: Id, txField: 'categoryId' | 'payeeId' | 'tagId') => {
      queryClient.setQueryData<EntityDto[]>(key, (prev) => (prev ?? []).filter((i) => i.id !== id))
      queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (prev) =>
        (prev ?? []).map((t) => (t[txField] === id ? { ...t, [txField]: null } : t)),
      )
      if (touchesBudget) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      }
    },
    replaceAll: (items: EntityDto[]) => queryClient.setQueryData(key, items),
  }
}

export function useUpdateCategory() {
  const ops = useEntityCacheOps('categories', true)
  return useMutation({
    mutationFn: (form: { id: Id; name: string; icon: string }) => categoryApi.updateCategory(form),
    onSuccess: (_r, form) => {
      ops.replaceItem(form.id, { name: form.name, icon: form.icon })
      trackEvent(METRICS.CATEGORY_UPDATE)
    },
  })
}

export function useArchiveCategory() {
  const ops = useEntityCacheOps('categories', true)
  return useMutation({
    mutationFn: (id: Id) => categoryApi.archiveCategory(id),
    onSuccess: (_r, id) => {
      ops.setArchived(id, 1)
      trackEvent(METRICS.CATEGORY_ARCHIVE)
    },
  })
}

export function useUnarchiveCategory() {
  const ops = useEntityCacheOps('categories', true)
  return useMutation({
    mutationFn: (id: Id) => categoryApi.unarchiveCategory(id),
    onSuccess: (_r, id) => {
      ops.setArchived(id, 0)
      trackEvent(METRICS.CATEGORY_UNARCHIVE)
    },
  })
}

export function useDeleteCategory() {
  const ops = useEntityCacheOps('categories', true)
  return useMutation({
    mutationFn: (id: Id) => categoryApi.deleteCategory(id),
    onSuccess: (_r, id) => {
      ops.remove(id, 'categoryId')
      trackEvent(METRICS.CATEGORY_DELETE)
    },
  })
}

export function useOrderCategories() {
  const ops = useEntityCacheOps('categories', false)
  return useMutation({
    mutationFn: categoryApi.orderCategoryList,
    onSuccess: (items) => {
      ops.replaceAll(items)
      trackEvent(METRICS.CATEGORY_ORDER_LIST)
    },
  })
}

export function useUpdatePayee() {
  const ops = useEntityCacheOps('payees', false)
  return useMutation({
    mutationFn: (form: { id: Id; name: string }) => payeeApi.updatePayee(form),
    onSuccess: (_r, form) => {
      ops.replaceItem(form.id, { name: form.name })
      trackEvent(METRICS.PAYEE_UPDATE)
    },
  })
}

export function useArchivePayee() {
  const ops = useEntityCacheOps('payees', false)
  return useMutation({
    mutationFn: (id: Id) => payeeApi.archivePayee(id),
    onSuccess: (_r, id) => {
      ops.setArchived(id, 1)
      trackEvent(METRICS.PAYEE_ARCHIVE)
    },
  })
}

export function useUnarchivePayee() {
  const ops = useEntityCacheOps('payees', false)
  return useMutation({
    mutationFn: (id: Id) => payeeApi.unarchivePayee(id),
    onSuccess: (_r, id) => {
      ops.setArchived(id, 0)
      trackEvent(METRICS.PAYEE_UNARCHIVE)
    },
  })
}

export function useDeletePayee() {
  const ops = useEntityCacheOps('payees', false)
  return useMutation({
    mutationFn: (id: Id) => payeeApi.deletePayee(id),
    onSuccess: (_r, id) => {
      ops.remove(id, 'payeeId')
      trackEvent(METRICS.PAYEE_DELETE)
    },
  })
}

export function useOrderPayees() {
  const ops = useEntityCacheOps('payees', false)
  return useMutation({
    mutationFn: payeeApi.orderPayeeList,
    onSuccess: (items) => {
      ops.replaceAll(items)
      trackEvent(METRICS.PAYEE_ORDER_LIST)
    },
  })
}

export function useUpdateTag() {
  const ops = useEntityCacheOps('tags', true)
  return useMutation({
    mutationFn: (form: { id: Id; name: string }) => tagApi.updateTag(form),
    onSuccess: (item, form) => {
      ops.replaceItem(form.id, { name: item?.name ?? form.name })
      trackEvent(METRICS.TAG_UPDATE)
    },
  })
}

export function useArchiveTag() {
  const ops = useEntityCacheOps('tags', true)
  return useMutation({
    mutationFn: (id: Id) => tagApi.archiveTag(id),
    onSuccess: (_r, id) => {
      ops.setArchived(id, 1)
      trackEvent(METRICS.TAG_ARCHIVE)
    },
  })
}

export function useUnarchiveTag() {
  const ops = useEntityCacheOps('tags', true)
  return useMutation({
    mutationFn: (id: Id) => tagApi.unarchiveTag(id),
    onSuccess: (_r, id) => {
      ops.setArchived(id, 0)
      trackEvent(METRICS.TAG_UNARCHIVE)
    },
  })
}

export function useDeleteTag() {
  const ops = useEntityCacheOps('tags', true)
  return useMutation({
    mutationFn: (id: Id) => tagApi.deleteTag(id),
    onSuccess: (_r, id) => {
      ops.remove(id, 'tagId')
      trackEvent(METRICS.TAG_DELETE)
    },
  })
}

export function useOrderTags() {
  const ops = useEntityCacheOps('tags', false)
  return useMutation({
    mutationFn: tagApi.orderTagList,
    onSuccess: (items) => {
      ops.replaceAll(items)
      trackEvent(METRICS.TAG_ORDER_LIST)
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
      const item = await tagApi.createTag({ id: uuidv7(), name: form.name, accountId: form.accountId })
      trackEvent(METRICS.TAG_CREATE)
      return item
    },
    onSuccess: (item) => {
      queryClient.setQueryData<TagDto[]>(queryKeys.tags, (prev) => {
        const items = prev ?? []
        return items.some((t) => t.id === item.id) ? items : [...items, item]
      })
    },
  })
}
