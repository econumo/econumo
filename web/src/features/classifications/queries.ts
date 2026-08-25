import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { v7 as uuidv7 } from 'uuid'
import * as categoryApi from '@/api/category'
import * as labelApi from '@/api/label'
import * as payeeApi from '@/api/payee'
import * as tagApi from '@/api/tag'
import type { CategoryDto, CategoryType } from '@/api/dto/category'
import type { LabelDto } from '@/api/dto/label'
import type { PayeeDto } from '@/api/dto/payee'
import type { RecurringDto } from '@/api/dto/recurring'
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

export function useLabels() {
  return useQuery({ queryKey: queryKeys.labels, queryFn: labelApi.getLabelList, staleTime: TEN_MINUTES, select: byPosition })
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

type EntityKind = 'categories' | 'payees' | 'tags' | 'labels'
type EntityDto = CategoryDto | PayeeDto | TagDto | LabelDto

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
      // The delete cascades ON DELETE SET NULL onto recurring templates too;
      // a stale id left in this cache would ride the next template edit (a
      // full-state replace) and be rejected as an unavailable item.
      queryClient.setQueryData<RecurringDto[]>(queryKeys.recurring, (prev) =>
        (prev ?? []).map((r) => (r[txField] === id ? { ...r, [txField]: null } : r)),
      )
      if (touchesBudget) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      }
    },
    // A merge RE-POINTS rows at the target rather than nulling them, which is
    // what separates it from remove() above. The budget is always invalidated:
    // the source's per-period limits were folded into the target's, so a cached
    // budget would still show the pre-merge numbers and read as data loss.
    merge: (sourceId: Id, targetId: Id, txField: 'categoryId' | 'payeeId' | 'tagId') => {
      queryClient.setQueryData<EntityDto[]>(key, (prev) => (prev ?? []).filter((i) => i.id !== sourceId))
      queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (prev) =>
        (prev ?? []).map((t) => (t[txField] === sourceId ? { ...t, [txField]: targetId } : t)),
      )
      queryClient.setQueryData<RecurringDto[]>(queryKeys.recurring, (prev) =>
        (prev ?? []).map((r) => (r[txField] === sourceId ? { ...r, [txField]: targetId } : r)),
      )
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
    },
    // Labels are many-to-many, so a merge rewrites the id sets: a row already
    // carrying both must end up with ONE target id, not a duplicate.
    mergeLabel: (sourceId: Id, targetId: Id) => {
      queryClient.setQueryData<EntityDto[]>(key, (prev) => (prev ?? []).filter((i) => i.id !== sourceId))
      const swap = (ids: Id[]) => (ids.includes(sourceId) ? [...new Set(ids.map((id) => (id === sourceId ? targetId : id)))] : ids)
      queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (prev) =>
        (prev ?? []).map((t) => ({ ...t, labelIds: swap(t.labelIds ?? []) })),
      )
      queryClient.setQueryData<RecurringDto[]>(queryKeys.recurring, (prev) =>
        (prev ?? []).map((r) => ({ ...r, labelIds: swap(r.labelIds ?? []) })),
      )
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
    },
    replaceAll: (items: EntityDto[]) => queryClient.setQueryData(key, items),
    // Reorder the cached list to an explicit id order and re-stamp dense
    // positions, so a bulk sort shows instantly instead of after the round trip.
    // Ids the cache does not know are ignored; rows the order omits keep their
    // slot at the end, matching how the server skips ids the caller does not own.
    reorder: (orderedIds: Id[]) => {
      queryClient.setQueryData<EntityDto[]>(key, (prev) => {
        if (!prev) {
          return prev
        }
        const byId = new Map(prev.map((i) => [i.id, i]))
        const ordered = orderedIds.map((id) => byId.get(id)).filter((i): i is EntityDto => i !== undefined)
        const seen = new Set(ordered.map((i) => i.id))
        return [...ordered, ...prev.filter((i) => !seen.has(i.id))].map((i, position) => ({ ...i, position }))
      })
    },
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

export function useMergeCategory() {
  const ops = useEntityCacheOps('categories', true)
  return useMutation({
    mutationFn: ({ sourceId, targetId }: { sourceId: Id; targetId: Id }) => categoryApi.mergeCategory(sourceId, targetId),
    onSuccess: (_r, { sourceId, targetId }) => {
      ops.merge(sourceId, targetId, 'categoryId')
      trackEvent(METRICS.CLASSIFICATION_MERGE, { type: 'category' })
    },
  })
}

export function useMoveCategory() {
  const ops = useEntityCacheOps('categories', false)
  return useMutation({
    mutationFn: ({ id, afterId }: { id: Id; afterId: Id | null }) => categoryApi.moveCategory(id, afterId),
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

export function useMergePayee() {
  const ops = useEntityCacheOps('payees', false)
  return useMutation({
    mutationFn: ({ sourceId, targetId }: { sourceId: Id; targetId: Id }) => payeeApi.mergePayee(sourceId, targetId),
    onSuccess: (_r, { sourceId, targetId }) => {
      ops.merge(sourceId, targetId, 'payeeId')
      trackEvent(METRICS.CLASSIFICATION_MERGE, { type: 'payee' })
    },
  })
}

export function useMovePayee() {
  const ops = useEntityCacheOps('payees', false)
  return useMutation({
    mutationFn: ({ id, afterId }: { id: Id; afterId: Id | null }) => payeeApi.movePayee(id, afterId),
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

export function useMergeTag() {
  const ops = useEntityCacheOps('tags', true)
  return useMutation({
    mutationFn: ({ sourceId, targetId }: { sourceId: Id; targetId: Id }) => tagApi.mergeTag(sourceId, targetId),
    onSuccess: (_r, { sourceId, targetId }) => {
      ops.merge(sourceId, targetId, 'tagId')
      trackEvent(METRICS.CLASSIFICATION_MERGE, { type: 'tag' })
    },
  })
}

export function useMoveTag() {
  const ops = useEntityCacheOps('tags', false)
  return useMutation({
    mutationFn: ({ id, afterId }: { id: Id; afterId: Id | null }) => tagApi.moveTag(id, afterId),
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

// useSortCategories applies an explicit order in ONE request. A drag is a relative
// move; sorting the whole list changes every row's neighbour, which no single
// move can express. The cache is reordered up front so the list flips instantly.
export function useSortCategories() {
  const ops = useEntityCacheOps('categories', false)
  return useMutation({
    mutationFn: (orderedIds: Id[]) => categoryApi.sortCategoryList(orderedIds),
    onMutate: (orderedIds) => {
      ops.reorder(orderedIds)
    },
    onSuccess: (items) => {
      ops.replaceAll(items)
      trackEvent(METRICS.CATEGORY_ORDER_LIST)
    },
  })
}

// useSortPayees applies an explicit order in ONE request. A drag is a relative
// move; sorting the whole list changes every row's neighbour, which no single
// move can express. The cache is reordered up front so the list flips instantly.
export function useSortPayees() {
  const ops = useEntityCacheOps('payees', false)
  return useMutation({
    mutationFn: (orderedIds: Id[]) => payeeApi.sortPayeeList(orderedIds),
    onMutate: (orderedIds) => {
      ops.reorder(orderedIds)
    },
    onSuccess: (items) => {
      ops.replaceAll(items)
      trackEvent(METRICS.PAYEE_ORDER_LIST)
    },
  })
}

// useSortTags applies an explicit order in ONE request. A drag is a relative
// move; sorting the whole list changes every row's neighbour, which no single
// move can express. The cache is reordered up front so the list flips instantly.
export function useSortTags() {
  const ops = useEntityCacheOps('tags', false)
  return useMutation({
    mutationFn: (orderedIds: Id[]) => tagApi.sortTagList(orderedIds),
    onMutate: (orderedIds) => {
      ops.reorder(orderedIds)
    },
    onSuccess: (items) => {
      ops.replaceAll(items)
      trackEvent(METRICS.TAG_ORDER_LIST)
    },
  })
}

export function useUpdateLabel() {
  const ops = useEntityCacheOps('labels', true)
  return useMutation({
    mutationFn: (form: { id: Id; name: string }) => labelApi.updateLabel(form),
    onSuccess: (item, form) => {
      ops.replaceItem(form.id, { name: item?.name ?? form.name })
      trackEvent(METRICS.LABEL_UPDATE)
    },
  })
}

export function useArchiveLabel() {
  const ops = useEntityCacheOps('labels', true)
  return useMutation({
    mutationFn: (id: Id) => labelApi.archiveLabel(id),
    onSuccess: (_r, id) => {
      ops.setArchived(id, 1)
      trackEvent(METRICS.LABEL_ARCHIVE)
    },
  })
}

export function useUnarchiveLabel() {
  const ops = useEntityCacheOps('labels', true)
  return useMutation({
    mutationFn: (id: Id) => labelApi.unarchiveLabel(id),
    onSuccess: (_r, id) => {
      ops.setArchived(id, 0)
      trackEvent(METRICS.LABEL_UNARCHIVE)
    },
  })
}

// useEntityCacheOps.remove nulls a single SCALAR id, which is the wrong shape
// for the many-to-many labelIds: deleting a label cascades to
// transactions_labels / recurring_transactions_labels server-side, so every
// cached row must drop the id too. A stale id left behind would be resent by
// the next edit (both writes REPLACE the whole set) and rejected outright as
// an unavailable item, failing the save.
function withoutLabel<T extends { labelIds?: Id[] }>(rows: T[] | undefined, id: Id): T[] {
  return (rows ?? []).map((row) => (row.labelIds?.includes(id) ? { ...row, labelIds: row.labelIds.filter((l) => l !== id) } : row))
}

export function useDeleteLabel() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: Id) => labelApi.deleteLabel(id),
    onSuccess: (_r, id) => {
      queryClient.setQueryData<LabelDto[]>(queryKeys.labels, (prev) => (prev ?? []).filter((l) => l.id !== id))
      queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (prev) => withoutLabel(prev, id))
      queryClient.setQueryData<RecurringDto[]>(queryKeys.recurring, (prev) => withoutLabel(prev, id))
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      trackEvent(METRICS.LABEL_DELETE)
    },
  })
}

export function useMergeLabel() {
  const ops = useEntityCacheOps('labels', true)
  return useMutation({
    mutationFn: ({ sourceId, targetId }: { sourceId: Id; targetId: Id }) => labelApi.mergeLabel(sourceId, targetId),
    onSuccess: (_r, { sourceId, targetId }) => {
      ops.mergeLabel(sourceId, targetId)
      trackEvent(METRICS.CLASSIFICATION_MERGE, { type: 'label' })
    },
  })
}

export function useMoveLabel() {
  const ops = useEntityCacheOps('labels', false)
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, afterId }: { id: Id; afterId: Id | null }) => labelApi.moveLabel(id, afterId),
    onSuccess: (items) => {
      ops.replaceAll(items)
      // Unlike tags (whose budget order lives on the elements), the budget's
      // labels block renders in label sort order, so a reorder changes it.
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      trackEvent(METRICS.LABEL_ORDER_LIST)
    },
  })
}

// useSortLabels applies an explicit order in ONE request. A drag is a relative
// move; sorting the whole list changes every row's neighbour, which no single
// move can express. The cache is reordered up front so the list flips instantly.
export function useSortLabels() {
  const ops = useEntityCacheOps('labels', false)
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (orderedIds: Id[]) => labelApi.sortLabelList(orderedIds),
    onMutate: (orderedIds) => {
      ops.reorder(orderedIds)
    },
    onSuccess: (items) => {
      ops.replaceAll(items)
      // Same rule as useMoveLabel: the budget's labels block follows label
      // sort order, so a reorder changes it.
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      trackEvent(METRICS.LABEL_ORDER_LIST)
    },
  })
}

// Tags and labels have INDEPENDENT name namespaces on the backend (see
// internal/label vs internal/tag): a label may share a name with a tag. The
// dedupe lookup below therefore reads ONLY the labels cache (queryKeys.labels),
// never the tags cache, so it can never resolve a label create to a tag's id.
export function useCreateLabel() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (form: { name: string; accountId?: Id; ownerUserId?: Id }) => {
      const existing = findByName(queryClient.getQueryData<LabelDto[]>(queryKeys.labels), form.name, form.ownerUserId)
      if (existing) {
        return existing
      }
      const item = await labelApi.createLabel({ id: uuidv7(), name: form.name, accountId: form.accountId })
      trackEvent(METRICS.LABEL_CREATE)
      return item
    },
    onSuccess: (item) => {
      queryClient.setQueryData<LabelDto[]>(queryKeys.labels, (prev) => {
        const items = prev ?? []
        return items.some((l) => l.id === item.id) ? items : [...items, item]
      })
    },
  })
}
