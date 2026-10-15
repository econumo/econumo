import { useTranslation } from 'react-i18next'
import type { ImportRunLinkDto } from '@/api/dto/imports'
import { RouterPage } from '@/app/router-pages'
import { Button } from '@/components/ui/button'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { apiErrorMessage } from '@/lib/apiError'
import { formatDate, formatDayHeading } from '@/lib/datetime'
import { moneyFormat } from '@/lib/money'
import { rowStatus } from './runRows'
import { useImportSourceHistory, useImportSources } from './queries'
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
                  <li key={row.id} className="flex items-center justify-between gap-3 rounded-lg bg-econumo-card px-4 py-3 text-sm">
                    <div className="min-w-0">
                      <div className={status === 'deleted' ? 'truncate text-muted-foreground line-through' : 'truncate'}>{row.externalPayee || row.externalTransactionId}</div>
                      <div className="truncate text-xs text-muted-foreground">
                        {cardName(row.externalAccountId)} · {localTime(row.externalPostedAt)} · {t(`imports.runs.row_status.${status}`)}
                      </div>
                    </div>
                    <div className="shrink-0 tabular-nums">{moneyFormat(row.externalAmount, null, { showCurrency: false, useNativePrecision: false })} {row.externalCurrency}</div>
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
    </SettingsShell>
  )
}
