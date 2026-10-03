import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { Command, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { useUiStore } from '@/app/uiStore'
import { RouterPage } from '@/app/router-pages'
import { METRICS, trackEvent } from '@/lib/metrics'
import type { ClassificationType } from '@/lib/search'
import { useFolders } from '@/features/accounts/queries'
import { canWriteToAccount } from '@/features/connections/shared'
import { canTouchTransaction } from '@/features/transactions/canTouchTransaction'
import { useDeleteTransaction, useTransactions } from '@/features/transactions/queries'
import { separatorText, type DailyListEntry, type ViewTransaction } from '@/features/transactions/useAccountTransactions'
import { ViewTransactionDialog } from '@/features/transactions/ViewTransactionDialog'
import { useUserData } from '@/features/user/queries'
import { AccountResult, ClassificationResult, TransactionResult, type ClassificationItem } from './SearchRows'
import { useGlobalSearch, type SearchScope } from './useGlobalSearch'
import { useWindowed } from './useWindowed'

const ALL: SearchScope = { kind: 'all' }
const GROUP_CAP = 5

type ClassificationGroup = 'categories' | 'payees' | 'tags' | 'labels'
type GroupKey = 'accounts' | ClassificationGroup

const CLASSIFICATION_GROUPS: { key: ClassificationGroup; type: ClassificationType }[] = [
  { key: 'categories', type: 'category' },
  { key: 'payees', type: 'payee' },
  { key: 'tags', type: 'tag' },
  { key: 'labels', type: 'label' },
]

// The window counts transactions, not entries, so day separators never eat
// into a chunk; a separator is only emitted when a row follows it.
function firstTransactions(entries: DailyListEntry[], count: number): DailyListEntry[] {
  const out: DailyListEntry[] = []
  let seen = 0
  for (const entry of entries) {
    if (seen === count) {
      break
    }
    out.push(entry)
    if (entry.kind === 'transaction') {
      seen++
    }
  }
  return out
}

// Lives inside the dialog content, which unmounts on close: every open starts
// from a fresh query/scope, and nothing is computed while the search is closed.
function SearchPanel({ onPreview }: { onPreview: (tx: ViewTransaction) => void }) {
  const { t, i18n } = useTranslation()
  const close = useUiStore((s) => s.closeSearch)
  const navigate = useNavigate()
  const { data: user } = useUserData()
  const { data: transactions } = useTransactions()
  const { data: folders } = useFolders()

  const [query, setQuery] = useState('')
  const [scope, setScope] = useState<SearchScope>(ALL)
  const [expanded, setExpanded] = useState<Partial<Record<GroupKey, boolean>>>({})

  const result = useGlobalSearch(query, scope)
  const { count, hasMore, sentinelRef } = useWindowed(result.transactionCount, `${query}|${JSON.stringify(scope)}`)

  const select = (type: ClassificationType | 'account' | 'transaction') => trackEvent(METRICS.GLOBAL_SEARCH_SELECT, { type })

  const capped = <T,>(key: GroupKey, items: T[]): T[] => (expanded[key] ? items : items.slice(0, GROUP_CAP))
  const showAll = (key: GroupKey, total: number) =>
    total > GROUP_CAP && !expanded[key] ? (
      <CommandItem value={`show-all:${key}`} onSelect={() => setExpanded((e) => ({ ...e, [key]: true }))} className="text-muted-foreground">
        {t('search.show_all', { count: String(total) })}
      </CommandItem>
    ) : null

  // classification groups stay empty until the user data resolves (own-only filter)
  const loaded = user !== undefined && transactions !== undefined
  const nothing =
    loaded &&
    query.trim() !== '' &&
    result.transactionCount === 0 &&
    result.accounts.length === 0 &&
    CLASSIFICATION_GROUPS.every(({ key }) => result[key].length === 0)

  return (
    <Command shouldFilter={false} loop className="bg-transparent p-0">
      <CommandInput autoFocus value={query} onValueChange={setQuery} placeholder={t('search.placeholder')} />
      <CommandList className="mt-2 max-h-[70vh]">
        {result.accounts.length > 0 ? (
          <CommandGroup heading={t('search.groups.accounts')}>
            {capped('accounts', result.accounts).map((account) => (
              <AccountResult
                key={account.id}
                account={account}
                folderName={folders?.find((f) => f.id === account.folderId)?.name}
                onSelect={() => {
                  select('account')
                  close()
                  navigate(RouterPage.ACCOUNT(account.id))
                }}
              />
            ))}
            {showAll('accounts', result.accounts.length)}
          </CommandGroup>
        ) : null}
        {CLASSIFICATION_GROUPS.map(({ key, type }) => {
          const items: ClassificationItem[] = result[key]
          return items.length > 0 ? (
            <CommandGroup key={key} heading={t(`search.groups.${key}`)}>
              {capped(key, items).map((item) => (
                <ClassificationResult
                  key={item.id}
                  type={type}
                  item={item}
                  onSelect={() => {
                    select(type)
                    setScope({ kind: type, id: item.id })
                    setQuery('')
                  }}
                />
              ))}
              {showAll(key, items.length)}
            </CommandGroup>
          ) : null
        })}
        {result.transactionCount > 0 ? (
          // the recent feed (empty query) and a drill-down are one list, so no heading
          <CommandGroup heading={query.trim() && scope.kind === 'all' ? t('search.groups.transactions') : undefined}>
            {firstTransactions(result.transactions, count).map((entry) =>
              entry.kind === 'separator' ? (
                <div key={`sep-${entry.day}`} className="px-2 pb-1 pt-3 text-xs font-medium uppercase text-muted-foreground">
                  {separatorText(entry, t, i18n.language)}
                </div>
              ) : (
                <TransactionResult
                  key={entry.transaction.id}
                  transaction={entry.transaction}
                  onSelect={() => {
                    select('transaction')
                    onPreview(entry.transaction)
                  }}
                />
              ),
            )}
            {hasMore ? <div ref={sentinelRef} aria-hidden="true" className="h-px" /> : null}
          </CommandGroup>
        ) : null}
        {nothing ? <div className="py-6 text-center text-sm text-muted-foreground">{t('search.nothing_found')}</div> : null}
      </CommandList>
    </Command>
  )
}

export function GlobalSearchDialog() {
  const { t } = useTranslation()
  const open = useUiStore((s) => s.searchOpen)
  const close = useUiStore((s) => s.closeSearch)
  const openTransactionModal = useUiStore((s) => s.openTransactionModal)
  const { data: user } = useUserData()
  const deleteTransaction = useDeleteTransaction()
  const [preview, setPreview] = useState<ViewTransaction | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ViewTransaction | null>(null)

  const canChange = (tx: ViewTransaction) => canTouchTransaction(tx, !!tx.account && canWriteToAccount(tx.account, user?.id))

  return (
    <>
      <ResponsiveDialog
        open={open}
        onOpenChange={(o) => !o && close()}
        title={t('search.title')}
        hideHeader
        fullScreen
        // interactions inside the stacked preview/confirm must not dismiss the search
        dismissible={!preview && !deleteTarget}
      >
        <SearchPanel onPreview={setPreview} />
      </ResponsiveDialog>

      {preview ? (
        <ViewTransactionDialog
          transaction={preview}
          onClose={() => setPreview(null)}
          onEdit={() => {
            setPreview(null)
            // the form must not open stacked under the search sheet
            close()
            openTransactionModal({ transaction: preview })
          }}
          // the preview stays open under the confirm: unmount+mount in one tick
          // races Radix's aria-hidden bookkeeping, and cancel returns to the preview
          onDelete={() => setDeleteTarget(preview)}
          canChange={canChange(preview)}
          isShared={(preview.account?.sharedAccess.length ?? 0) > 0}
          dismissible={deleteTarget === null}
        />
      ) : null}

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (deleteTarget) {
            deleteTransaction.mutate(deleteTarget.id, {
              onSuccess: () => setPreview(null),
              onSettled: () => setDeleteTarget(null),
            })
          }
        }}
        question={t('accounts.page.delete_transaction_modal.question')}
        confirmLabel={t('common.button.delete.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </>
  )
}
