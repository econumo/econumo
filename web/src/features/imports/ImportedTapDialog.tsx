import { useTranslation } from 'react-i18next'
import type { ImportRunLinkDto } from '@/api/dto/imports'
import { InfoBox } from '@/components/InfoBox'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { Button } from '@/components/ui/button'
import { moneyFormat } from '@/lib/money'
import { rowStatus } from './runRows'
import { parseUtcDateTime } from './relativeTime'

const pad = (n: number) => String(n).padStart(2, '0')

interface ImportedTapDialogProps {
  row: ImportRunLinkDto | null
  cardName: string
  /** offered for a row that can still become a transaction (queued or skipped) */
  onCreate?: () => void
  onClose: () => void
}

// What the import history shows for a row with no transaction to preview: a
// tap that was skipped, is still waiting, or whose transaction was deleted.
export function ImportedTapDialog({ row, cardName, onCreate, onClose }: ImportedTapDialogProps) {
  const { t } = useTranslation()
  if (!row) {
    return null
  }
  const status = rowStatus(row)
  const posted = parseUtcDateTime(row.externalPostedAt)
  const when = `${posted.getFullYear()}-${pad(posted.getMonth() + 1)}-${pad(posted.getDate())} ${pad(posted.getHours())}:${pad(posted.getMinutes())}`
  const message =
    status === 'queued' ? t('imports.history.waiting')
      : status === 'deleted' ? t('imports.history.deleted')
        : status === 'imported' ? t('imports.history.unavailable')
          : t('imports.history.not_added')
  const details: [string, string][] = [
    [t('transactions.modal.form.amount.label'), `${moneyFormat(row.externalAmount, null, { showCurrency: false, useNativePrecision: false })} ${row.externalCurrency}`],
    [t('imports.history.card'), cardName],
    [t('imports.history.date'), when],
  ]
  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && onClose()} title={row.externalPayee || row.externalTransactionId}>
      <div className="flex flex-col gap-3">
        <InfoBox>{message}</InfoBox>
        <dl className="flex flex-col rounded-lg bg-econumo-card text-sm">
          {details.map(([label, value]) => (
            <div key={label} className="flex items-center justify-between gap-3 border-b border-background px-4 py-3 last:border-b-0">
              <dt className="text-muted-foreground">{label}</dt>
              <dd className="min-w-0 truncate tabular-nums">{value}</dd>
            </div>
          ))}
        </dl>
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>{t('common.button.close.label')}</Button>
          {onCreate ? <Button type="button" onClick={onCreate}>{t('imports.history.create')}</Button> : null}
        </div>
      </div>
    </ResponsiveDialog>
  )
}
