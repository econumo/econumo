import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { Command, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { useIsPhone } from '@/hooks/useIsPhone'
import { useUiStore } from '@/app/uiStore'
import { RouterPage } from '@/app/router-pages'
import { METRICS, trackEvent } from '@/lib/metrics'
import type { ClassificationType } from '@/lib/search'
import { useFolders } from '@/features/accounts/queries'
import { useConnections } from '@/features/connections/queries'
import { canWriteToAccount } from '@/features/connections/shared'
import { canTouchTransaction } from '@/features/transactions/canTouchTransaction'
import { useDeleteTransaction, useTransactions } from '@/features/transactions/queries'
import {
  separatorText,
  useTransactionLookups,
  type DailyListEntry,
  type ViewTransaction,
} from '@/features/transactions/useAccountTransactions'
import { ViewTransactionDialog } from '@/features/transactions/ViewTransactionDialog'
import { useUserData } from '@/features/user/queries'
import { AccountResult, ClassificationResult, DrillHeader, TransactionResult, type ClassificationItem } from './SearchRows'
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

const GROUP_OF: Record<ClassificationType, ClassificationGroup> = {
  category: 'categories',
  payee: 'payees',
  tag: 'tags',
  label: 'labels',
}

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
  const { data: connections } = useConnections()
  // with any connection, a cross-account list needs to say who spent
  const showAuthor = (connections?.length ?? 0) > 0
  const lookups = useTransactionLookups()
  const inputRef = useRef<HTMLInputElement>(null)

  const [query, setQuery] = useState('')
  const [scope, setScope] = useState<SearchScope>(ALL)
  const [prevQuery, setPrevQuery] = useState('')
  const [expanded, setExpanded] = useState<Partial<Record<GroupKey, boolean>>>({})

  const drillList: ClassificationItem[] | undefined = scope.kind === 'all' ? undefined : lookups[GROUP_OF[scope.kind]]
  const drilled = scope.kind === 'all' ? undefined : drillList?.find((i) => i.id === scope.id)

  // "Show all" belongs to the result set it was clicked on
  const changeQuery = (next: string) => {
    setQuery(next)
    setExpanded({})
  }
  const drillInto = (type: ClassificationType, id: string) => {
    setPrevQuery(query)
    setScope({ kind: type, id })
    changeQuery('')
  }
  const goBack = () => {
    setScope(ALL)
    changeQuery(prevQuery)
  }
  // the drilled item was deleted or merged away: adjusting state during render
  // (not in an effect) so a header for a vanished item never paints
  if (drillList && !drilled) {
    goBack()
  }

  // back/drill clicks take focus off the input, and the drill header unmounts with it
  useEffect(() => inputRef.current?.focus(), [scope])

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
  // the unfiltered recent feed is the one view where an empty list says nothing
  const nothing =
    loaded &&
    (query.trim() !== '' || scope.kind !== 'all') &&
    result.transactionCount === 0 &&
    result.accounts.length === 0 &&
    CLASSIFICATION_GROUPS.every(({ key }) => result[key].length === 0)

  // phones: the panel fills the full-screen sheet (the list takes the rest) and
  // its first row keeps clear of the corner close button
  return (
    <div className="flex flex-col max-sm:h-full max-sm:pt-[env(safe-area-inset-top)]">
      {/* outside Command: cmdk claims Enter on its root, which would hijack the Back button */}
      {drilled && scope.kind !== 'all' ? <DrillHeader type={scope.kind} item={drilled} onBack={goBack} className="max-sm:pr-8" /> : null}
      <Command shouldFilter={false} loop className="min-h-0 bg-transparent p-0 max-sm:flex-1">
        <div className={drilled ? undefined : 'max-sm:pr-8'}>
          <CommandInput
            ref={inputRef}
            autoFocus
            value={query}
            onValueChange={changeQuery}
            onKeyDown={(e) => {
              // a held Backspace clearing the query must not also leave the drill-down
              if (e.key === 'Backspace' && !e.repeat && query === '' && scope.kind !== 'all') {
                e.preventDefault()
                goBack()
              }
            }}
            placeholder={t('search.placeholder')}
          />
        </div>
        <CommandList className="mt-2 max-h-none max-sm:min-h-0 max-sm:flex-1 sm:max-h-[70vh]">
          {result.accounts.length > 0 ? (
            <CommandGroup heading={t('search.groups.accounts')}>
              {capped('accounts', result.accounts).map((account) => (
                <AccountResult
                  key={account.id}
                  account={account}
                  folderName={folders?.find((f) => f.id === account.folderId)?.name}
                  folderHidden={folders?.find((f) => f.id === account.folderId)?.isVisible === 0}
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
                      drillInto(type, item.id)
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
                    showAuthor={showAuthor}
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
    </div>
  )
}

export function GlobalSearchDialog() {
  const { t } = useTranslation()
  const open = useUiStore((s) => s.searchOpen)
  const close = useUiStore((s) => s.closeSearch)
  const openTransactionModal = useUiStore((s) => s.openTransactionModal)
  const { data: user } = useUserData()
  const deleteTransaction = useDeleteTransaction()
  const isPhone = useIsPhone()
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
        size="wide"
        // the full-screen phone sheet has no Escape key or overlay to leave by
        showClose={isPhone}
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
