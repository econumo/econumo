import type { ReactNode } from 'react'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { toast } from 'sonner'
import type { ImportFailedEventDto, ImportQueuedEventDto } from '@/api/dto/imports'
import { RouterPage } from '@/app/router-pages'
import { useUiStore } from '@/app/uiStore'
import { Button } from '@/components/ui/button'
import { useAccounts } from '@/features/accounts/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { formatDateTime, parseDateTime } from '@/lib/datetime'
import { moneyFormat } from '@/lib/money'
import { useDiscardImportEvent, useIgnoreImportAccount, useImportSources, useRetryImportEvent, useSkipQueuedEvent, useUnskipQueuedEvent } from './queries'

function groupByCard(rows: ImportQueuedEventDto[]): Map<string, ImportQueuedEventDto[]> {
  const out = new Map<string, ImportQueuedEventDto[]>()
  for (const row of rows) {
    const key = `${row.sourceId} ${row.externalAccountId}`
    out.set(key, [...(out.get(key) ?? []), row])
  }
  return out
}

function reasonText(t: TFunction, row: ImportQueuedEventDto): string {
  return row.reason === 'no_rate'
    ? t('imports.queue.reason.no_rate', { currency: row.currency })
    : row.reason === 'account_deleted'
      ? t('imports.queue.reason.account_deleted')
      : row.reason === 'no_access'
        ? t('imports.queue.reason.no_access')
        : t('imports.queue.reason.unmapped')
}

// no CurrencyLike (symbol/fractionDigits) travels with a queue row, only the
// ISO code, so moneyFormat gets no currency and the code is appended as text
function rowAmount(r: ImportQueuedEventDto): string {
  return `${r.type === 'expense' ? '-' : '+'}${moneyFormat(r.amount, null, { showCurrency: false, useNativePrecision: false })} ${r.currency}`
}

function QueueRow({ row, action, cardName }: { row: ImportQueuedEventDto; action: ReactNode; cardName?: string }) {
  const { t } = useTranslation()
  const openTransactionModal = useUiStore((s) => s.openTransactionModal)
  const review = () =>
    openTransactionModal({
      importQueued: { linkId: row.linkId, type: row.type, accountId: row.accountId, amount: row.amount, currency: row.currency, payee: row.payee, date: row.postedAt },
    })
  return (
    <div className="flex items-center gap-3 rounded-lg bg-econumo-card px-4 py-3 text-sm">
      <button type="button" className="min-w-0 flex-1 text-left" title={t('imports.queue.import')} onClick={review}>
        <span className="block truncate font-medium">{row.payee}</span>
        <span className="block text-xs text-muted-foreground">
          {cardName ? `${cardName} · ` : ''}{formatDateTime(parseDateTime(row.postedAt))} · {reasonText(t, row)}
        </span>
      </button>
      <div className={`shrink-0 ${row.type === 'expense' ? 'text-expense' : 'text-income'}`}>{rowAmount(row)}</div>
      {action}
    </div>
  )
}

export function ToReviewSection({ queued }: { queued: ImportQueuedEventDto[] }) {
  const { t } = useTranslation()
  const { data: sources = [] } = useImportSources()
  const { data: accounts = [] } = useAccounts()
  const skip = useSkipQueuedEvent()
  const ignore = useIgnoreImportAccount()
  const onError = (err: unknown) => toast.error(apiErrorMessage(err))

  const cardState = (row: ImportQueuedEventDto) =>
    sources.find((s) => s.id === row.sourceId)?.cards.find((c) => c.externalAccountId === row.externalAccountId)?.state ?? 'unmapped'
  const accountName = (id: string) => accounts.find((a) => a.id === id)?.name ?? ''

  return (
    <div className="flex flex-col gap-2">
      {[...groupByCard(queued).entries()].map(([key, rows]) => {
        const first = rows[0]
        const state = cardState(first)
        return (
          <div key={key} className="flex flex-col gap-2">
            <div className="flex items-center justify-between gap-2 px-1 pt-2 text-sm">
              <span className="font-medium">{first.externalAccountId}</span>
              {state === 'unmapped' ? (
                <span className="flex gap-3">
                  <Link to={RouterPage.SETTINGS_APPLE_WALLET} className="text-primary underline-offset-2 hover:underline">{t('imports.apple_wallet.cards.map')}</Link>
                  <button type="button" className="text-muted-foreground underline-offset-2 hover:underline"
                    disabled={ignore.isPending && ignore.variables?.sourceId === first.sourceId && ignore.variables?.externalAccountId === first.externalAccountId}
                    onClick={() => ignore.mutate({ sourceId: first.sourceId, externalAccountId: first.externalAccountId }, { onError })}>
                    {t('imports.apple_wallet.cards.ignore')}
                  </button>
                </span>
              ) : first.accountId ? (
                <span className="text-xs text-muted-foreground">{accountName(first.accountId)}</span>
              ) : null}
            </div>
            {rows.map((r) => (
              <QueueRow
                key={r.linkId}
                row={r}
                action={
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    aria-label={`${t('imports.queue.skip')} ${r.payee}`}
                    disabled={skip.isPending && skip.variables === r.linkId}
                    onClick={() => skip.mutate(r.linkId, { onError })}
                  >
                    {t('imports.queue.skip')}
                  </Button>
                }
              />
            ))}
          </div>
        )
      })}
    </div>
  )
}

export function SkippedSection({ skipped }: { skipped: ImportQueuedEventDto[] }) {
  const { t } = useTranslation()
  const unskip = useUnskipQueuedEvent()
  const onError = (err: unknown) => toast.error(apiErrorMessage(err))
  return (
    <div className="flex flex-col gap-2">
      {skipped.map((r) => (
        <QueueRow
          key={r.linkId}
          row={r}
          cardName={r.externalAccountId}
          action={
            <Button
              type="button"
              size="sm"
              variant="ghost"
              aria-label={`${t('imports.queue.unskip')} ${r.payee}`}
              disabled={unskip.isPending && unskip.variables === r.linkId}
              onClick={() => unskip.mutate(r.linkId, { onError })}
            >
              {t('imports.queue.unskip')}
            </Button>
          }
        />
      ))}
    </div>
  )
}

export function FailedImportsSection({ failed }: { failed: ImportFailedEventDto[] }) {
  const { t } = useTranslation()
  const retry = useRetryImportEvent()
  const discard = useDiscardImportEvent()
  const onError = (err: unknown) => toast.error(apiErrorMessage(err))

  const retryResultText = (status: string) =>
    status === 'created'
      ? t('imports.queue.retry_result.created')
      : status === 'queued'
        ? t('imports.queue.retry_result.queued')
        : status === 'skipped'
          ? t('imports.queue.retry_result.skipped')
          : status === 'duplicate'
            ? t('imports.queue.retry_result.duplicate')
            : t('imports.queue.retry_result.failed')

  const retryEvent = (ev: ImportFailedEventDto) =>
    retry.mutate(ev.eventId, {
      onSuccess: (result) => toast.success(retryResultText(result.status)),
      onError,
    })

  return (
    <div className="flex flex-col gap-2">
      {failed.map((ev) => (
        <div key={ev.eventId} className="flex flex-col gap-2 rounded-lg bg-econumo-card px-4 py-3 text-sm">
          <div className="text-xs text-muted-foreground">{t('imports.queue.received', { date: formatDateTime(parseDateTime(ev.receivedAt)) })}</div>
          <div className="text-destructive">{ev.error}</div>
          <pre className="overflow-x-auto whitespace-pre-wrap break-all text-xs text-muted-foreground">{ev.payload}</pre>
          <div className="flex gap-2">
            <Button
              type="button"
              size="sm"
              variant="secondary"
              aria-label={`${t('imports.queue.retry')} ${t('imports.queue.received', { date: formatDateTime(parseDateTime(ev.receivedAt)) })}`}
              disabled={retry.isPending && retry.variables === ev.eventId}
              onClick={() => retryEvent(ev)}
            >
              {t('imports.queue.retry')}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              aria-label={`${t('imports.queue.discard')} ${t('imports.queue.received', { date: formatDateTime(parseDateTime(ev.receivedAt)) })}`}
              disabled={discard.isPending && discard.variables === ev.eventId}
              onClick={() => discard.mutate(ev.eventId, { onError })}
            >
              {t('imports.queue.discard')}
            </Button>
          </div>
        </div>
      ))}
    </div>
  )
}
