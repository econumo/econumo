import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router'
import type { ImportRunLinkDto } from '@/api/dto/imports'
import { RouterPage } from '@/app/router-pages'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { apiErrorMessage } from '@/lib/apiError'
import { dayKey, formatDayHeading } from '@/lib/datetime'
import { moneyFormat } from '@/lib/money'
import { ImportRunSummary } from './ImportRunSummary'
import { useImportRun, useImportSources } from './queries'

// A linked row whose transaction is gone is a tombstone: the import happened,
// the user deleted the result afterwards. It stays listed so the history is
// honest, and so a future "allow re-import" action has something to act on.
function rowStatus(link: ImportRunLinkDto): 'imported' | 'deleted' | 'queued' | 'skipped' {
  if (link.status === 'linked') {
    return link.transactionId ? 'imported' : 'deleted'
  }
  return link.status === 'queued' ? 'queued' : 'skipped'
}

export function ImportRunPage() {
  const { t, i18n } = useTranslation()
  const { id = '' } = useParams()
  const detail = useImportRun(id)
  const { data: sources = [] } = useImportSources()
  const source = sources.find((s) => s.id === detail.data?.item.sourceId)
  // A bridge account id ("ACT-…") means nothing to the user; the source's
  // cards carry the bank's own name for it.
  const accountName = (externalAccountId: string) =>
    source?.cards.find((c) => c.externalAccountId === externalAccountId)?.externalName || externalAccountId
  return (
    <SettingsShell title={t('imports.runs.detail_header')} backTo={RouterPage.IMPORT_RUNS}>
      <div className="mx-auto flex w-full max-w-xl flex-col gap-2">
        {detail.isPending ? <p className="text-sm text-muted-foreground">{t('common.app.modal.loading.data_loading')}</p> : null}
        {detail.isError ? <p role="alert" className="text-sm text-destructive">{apiErrorMessage(detail.error)}</p> : null}
        {detail.data ? (
          <>
            <ImportRunSummary run={detail.data.item} accountName={accountName} />
            <p className="px-1 pt-2 text-xs uppercase text-muted-foreground">{t('imports.runs.rows_header')}</p>
            {detail.data.links.length === 0 ? (
              <p className="rounded-lg bg-econumo-card px-4 py-3.5 text-sm text-muted-foreground">{t('imports.runs.rows_empty')}</p>
            ) : detail.data.links.map((link) => {
              const status = rowStatus(link)
              return (
                <div key={link.id} className="flex items-center justify-between gap-2 rounded-lg bg-econumo-card px-4 py-3.5 text-sm">
                  <div className="min-w-0">
                    <div className={status === 'deleted' ? 'truncate line-through text-muted-foreground' : 'truncate'}>{link.externalPayee || link.externalTransactionId}</div>
                    <div className="text-xs text-muted-foreground">
                      {accountName(link.externalAccountId)} · {formatDayHeading(dayKey(link.externalPostedAt), i18n.language)} · {t(`imports.runs.row_status.${status}`)}
                    </div>
                  </div>
                  <div className="shrink-0 tabular-nums">{moneyFormat(link.externalAmount, null, { showCurrency: false, useNativePrecision: false })} {link.externalCurrency}</div>
                </div>
              )
            })}
          </>
        ) : null}
      </div>
    </SettingsShell>
  )
}
