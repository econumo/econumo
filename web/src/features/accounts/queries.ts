import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as accountApi from '@/api/account'
import type { AccountDto, AccountItemDto } from '@/api/dto/account'
import type { FolderDto } from '@/api/dto/folder'
import type { TransactionDto } from '@/api/dto/transaction'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'

export function useAccounts() {
  return useQuery({
    queryKey: queryKeys.accounts,
    queryFn: accountApi.getAccountList,
    staleTime: TEN_MINUTES,
    // get-account-list response order differs from order-account-list; position is authoritative
    select: (items) => [...items].sort((a, b) => a.position - b.position),
  })
}

export function useFolders() {
  return useQuery({
    queryKey: queryKeys.folders,
    queryFn: accountApi.getFolderList,
    staleTime: TEN_MINUTES,
    select: (items) => [...items].sort((a, b) => a.position - b.position),
  })
}

function upsert<T extends { id: string }>(list: T[] | undefined, item: T): T[] {
  const items = list ?? []
  const idx = items.findIndex((x) => x.id === item.id)
  if (idx === -1) {
    return [...items, item]
  }
  const next = [...items]
  next[idx] = item
  return next
}

export function useAccountItemEffects() {
  const queryClient = useQueryClient()
  return (result: AccountItemDto, opts: { checkFirstFolder?: boolean } = {}) => {
    queryClient.setQueryData<AccountDto[]>(queryKeys.accounts, (prev) => upsert(prev, result.item))
    if (result.transaction) {
      queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (prev) => [result.transaction as TransactionDto, ...(prev ?? [])])
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
    }
    if (opts.checkFirstFolder) {
      const folders = queryClient.getQueryData<FolderDto[]>(queryKeys.folders)
      if (!folders || folders.length === 0) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.folders })
      }
    }
  }
}

export function useCreateAccount() {
  const applyItem = useAccountItemEffects()
  return useMutation({
    mutationFn: accountApi.createAccount,
    onSuccess: (result) => {
      applyItem(result, { checkFirstFolder: true })
      trackEvent(METRICS.ACCOUNT_CREATE)
    },
  })
}

export function useUpdateAccount() {
  const applyItem = useAccountItemEffects()
  return useMutation({
    mutationFn: accountApi.updateAccount,
    onSuccess: (result) => {
      applyItem(result)
      trackEvent(METRICS.ACCOUNT_UPDATE)
    },
  })
}

export function useDeleteAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: accountApi.deleteAccount,
    onSuccess: (_result, id) => {
      queryClient.setQueryData<AccountDto[]>(queryKeys.accounts, (prev) => (prev ?? []).filter((a) => a.id !== id))
      queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (prev) =>
        (prev ?? []).filter((t) => t.accountId !== id && t.accountRecipientId !== id),
      )
    },
  })
}

export function useOrderAccounts() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: accountApi.orderAccountList,
    // optimistic: the new arrangement lands instantly, the echo just confirms it
    onMutate: async (changes) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.accounts })
      const previous = queryClient.getQueryData<AccountDto[]>(queryKeys.accounts)
      const byId = new Map(changes.map((c) => [c.id, c]))
      queryClient.setQueryData<AccountDto[]>(queryKeys.accounts, (prev) =>
        (prev ?? []).map((a) => {
          const change = byId.get(a.id)
          return change ? { ...a, folderId: change.folderId, position: change.position } : a
        }),
      )
      return { previous }
    },
    onError: (_err, _changes, context) => {
      if (context?.previous) {
        queryClient.setQueryData(queryKeys.accounts, context.previous)
      }
    },
    onSuccess: (items) => queryClient.setQueryData(queryKeys.accounts, items),
  })
}

export function useCreateFolder() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: accountApi.createFolder,
    onSuccess: (item) => queryClient.setQueryData<FolderDto[]>(queryKeys.folders, (prev) => upsert(prev, item)),
  })
}

export function useUpdateFolder() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) => accountApi.updateFolder(id, name),
    onSuccess: (item) => queryClient.setQueryData<FolderDto[]>(queryKeys.folders, (prev) => upsert(prev, item)),
  })
}

export function useOrderFolders() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: accountApi.orderFolderList,
    onMutate: async (changes) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.folders })
      const previous = queryClient.getQueryData<FolderDto[]>(queryKeys.folders)
      const positions = new Map(changes.map((c) => [c.id, c.position]))
      queryClient.setQueryData<FolderDto[]>(queryKeys.folders, (prev) =>
        (prev ?? []).map((f) => {
          const position = positions.get(f.id)
          return position === undefined ? f : { ...f, position }
        }),
      )
      return { previous }
    },
    onError: (_err, _changes, context) => {
      if (context?.previous) {
        queryClient.setQueryData(queryKeys.folders, context.previous)
      }
    },
    onSuccess: (items) => queryClient.setQueryData(queryKeys.folders, items),
  })
}

export function useReplaceFolder() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, replaceId }: { id: string; replaceId: string }) => accountApi.replaceFolder(id, replaceId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.folders })
      void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
    },
  })
}

function useSetFolderVisibility(mutationFn: (id: string) => Promise<void>, isVisible: 0 | 1) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn,
    onMutate: async (id: string) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.folders })
      const previous = queryClient.getQueryData<FolderDto[]>(queryKeys.folders)
      queryClient.setQueryData<FolderDto[]>(queryKeys.folders, (prev) =>
        (prev ?? []).map((f) => (f.id === id ? { ...f, isVisible } : f)),
      )
      return { previous }
    },
    onError: (_err, _id, context) => {
      if (context?.previous) {
        queryClient.setQueryData(queryKeys.folders, context.previous)
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.folders })
      void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
    },
  })
}

export function useHideFolder() {
  return useSetFolderVisibility(accountApi.hideFolder, 0)
}

export function useShowFolder() {
  return useSetFolderVisibility(accountApi.showFolder, 1)
}
