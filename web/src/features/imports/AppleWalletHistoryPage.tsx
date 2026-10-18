import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ImportRunLinkDto } from '@/api/dto/imports'
import { RouterPage } from '@/app/router-pages'
import { useUiStore } from '@/app/uiStore'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { Button } from '@/components/ui/button'
import { canWriteToAccount } from '@/features/connections/shared'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { canTouchTransaction } from '@/features/transactions/canTouchTransaction'
import { useDeleteTransaction, useTransactions } from '@/features/transactions/queries'
import { enrichTransaction, useTransactionLookups, type ViewTransaction } from '@/features/transactions/useAccountTransactions'
import { ViewTransactionDialog } from '@/features/transactions/ViewTransactionDialog'
import { useUserData } from '@/features/user/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { formatDate, formatDayHeading } from '@/lib/datetime'
import { moneyFormat } from '@/lib/money'
import { ImportedTapDialog } from './ImportedTapDialog'
import { rowStatus } from './runRows'
import { useImportQueue, useImportSourceHistory, useImportSources } from './queries'
import { parseUtcDateTime } from './relativeTime'

const pad = (n: number) => String(n).padStart(2, '0')

// A tap's time is stored in UTC; the user reads it on their own clock.
function localDay(posted: string): string {
  return formatDate(parseUtcDateTime(posted))
}

function localTime(posted: string): string {
  const d = parseUtcDateTime(posted)
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function AppleWalletHistoryPage() {
  const { t, i18n } = useTranslation()
  const { data: sources } = useImportSources()
  const wallet = sources?.find((s) => s.provider === 'apple-wallet')
  const history = useImportSourceHistory(wallet?.id)
  const rows = history.data?.pages.flatMap((p) => p.items) ?? []
  const cardName = (externalAccountId: string) =>
    wallet?.cards.find((c) => c.externalAccountId === externalAccountId)?.externalName || externalAccountId
  const { data: transactions, isPending: transactionsLoading } = useTransactions()
  const lookups = useTransactionLookups()
  const { data: user } = useUserData()
  const { data: queue } = useImportQueue()
  const openTransactionModal = useUiStore((s) => s.openTransactionModal)
  const deleteTransaction = useDeleteTransaction()
  const [preview, setPreview] = useState<ViewTransaction | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ViewTransaction | null>(null)
  const [detail, setDetail] = useState<ImportRunLinkDto | null>(null)
  // an imported tap tapped before the transaction list has loaded opens once it has
  const [pending, setPending] = useState<ImportRunLinkDto | null>(null)

  // An imported tap opens the same preview as any transaction; anything else
  // (skipped, still waiting, deleted since) shows what was received.
  const open = (row: ImportRunLinkDto) => {
    if (row.transactionId && transactionsLoading) {
      setPending(row)
      return
    }
    const tx = row.transactionId ? transactions?.find((x) => x.id === row.transactionId) : undefined
    if (tx) {
      setPreview(enrichTransaction(tx, lookups))
    } else {
      setDetail(row)
    }
  }
  useEffect(() => {
    if (pending && !transactionsLoading) {
      setPending(null)
      open(pending)
    }
    // open reads the latest list; re-running on its identity would loop
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pending, transactionsLoading])
  // queued and skipped rows are in the Inbox queue, which carries what the
  // transaction form needs (type, mapped account); creating one links it
  const queued = detail ? [...(queue?.queued ?? []), ...(queue?.skipped ?? [])].find((q) => q.linkId === detail.id) : undefined
  const createFromDetail = queued
    ? () => {
        openTransactionModal({
          importQueued: { linkId: queued.linkId, type: queued.type, accountId: queued.accountId, amount: queued.amount, currency: queued.currency, payee: queued.payee, date: queued.postedAt },
        })
        setDetail(null)
      }
    : undefined

  const days: [string, ImportRunLinkDto[]][] = []
  for (const row of rows) {
    const day = localDay(row.externalPostedAt)
    const last = days.at(-1)
    if (last && last[0] === day) {
      last[1].push(row)
    } else {
      days.push([day, [row]])
    }
  }

  return (
    <SettingsShell
      title={t('imports.runs.header')}
      backTo={RouterPage.SETTINGS_APPLE_WALLET}
      crumbs={[
        { label: t('settings.page.header_desktop'), to: RouterPage.SETTINGS },
        { label: t('imports.apple_wallet.header'), to: RouterPage.SETTINGS_APPLE_WALLET },
      ]}
    >
      <div className="mx-auto flex w-full max-w-xl flex-col gap-2">
        {history.isPending && wallet ? <p className="text-sm text-muted-foreground">{t('common.app.modal.loading.data_loading')}</p> : null}
        {history.isError ? <p role="alert" className="text-sm text-destructive">{apiErrorMessage(history.error)}</p> : null}
        {sources && (!wallet || (history.isSuccess && rows.length === 0)) ? (
          <p className="rounded-lg bg-econumo-card px-4 py-3.5 text-sm text-muted-foreground">{t('imports.runs.empty')}</p>
        ) : null}
        {days.map(([day, items]) => (
          <section key={day} className="flex flex-col gap-2">
            <h2 className="px-1 pt-2 text-xs uppercase text-muted-foreground">{formatDayHeading(day, i18n.language)}</h2>
            <ul className="flex flex-col gap-2">
              {items.map((row) => {
                const status = rowStatus(row)
                return (
                  <li key={row.id}>
                    <button type="button" className="flex w-full items-center justify-between gap-3 rounded-lg bg-econumo-card px-4 py-3 text-left text-sm hover:bg-econumo-hover" onClick={() => open(row)}>
                      <div className="min-w-0">
                        <div className={status === 'deleted' ? 'truncate text-muted-foreground line-through' : 'truncate'}>{row.externalPayee || row.externalTransactionId}</div>
                        <div className="truncate text-xs text-muted-foreground">
                          {cardName(row.externalAccountId)} · {localTime(row.externalPostedAt)} · {t(`imports.runs.row_status.${status}`)}
                        </div>
                      </div>
                      <div className="shrink-0 tabular-nums">{moneyFormat(row.externalAmount, null, { showCurrency: false, useNativePrecision: false })} {row.externalCurrency}</div>
                    </button>
                  </li>
                )
              })}
            </ul>
          </section>
        ))}
        {history.hasNextPage ? (
          <Button type="button" variant="secondary" className="self-center" disabled={history.isFetchingNextPage} onClick={() => void history.fetchNextPage()}>
            {t('imports.runs.load_more')}
          </Button>
        ) : null}
      </div>
      {preview ? (
        <ViewTransactionDialog
          transaction={preview}
          onClose={() => setPreview(null)}
          onEdit={() => {
            openTransactionModal({ transaction: preview })
            setPreview(null)
          }}
          onDelete={() => setDeleteTarget(preview)}
          canChange={canTouchTransaction(preview, !!preview.account && canWriteToAccount(preview.account, user?.id))}
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
      <ImportedTapDialog row={detail} cardName={detail ? cardName(detail.externalAccountId) : ''} onCreate={createFromDetail} onClose={() => setDetail(null)} />
    </SettingsShell>
  )
}
