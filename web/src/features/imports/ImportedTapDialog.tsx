import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { ImportRunLinkDto } from '@/api/dto/imports'
import { InfoBox } from '@/components/InfoBox'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { Button } from '@/components/ui/button'
import { useOnline } from '@/hooks/useOnline'
import { apiErrorMessage } from '@/lib/apiError'
import { parseDateTime } from '@/lib/datetime'
import { moneyFormat } from '@/lib/money'
import { useReimportImportLink } from './queries'
import { rowStatus } from './runRows'

const pad = (n: number) => String(n).padStart(2, '0')

interface ImportedTapDialogProps {
  row: ImportRunLinkDto | null
  cardName: string
  /** the label of the cardName line: a Wallet card, or a bank account */
  cardLabel?: string
  /** offered for a row that can still become a transaction (queued or skipped) */
  onCreate?: () => void
  onClose: () => void
}

// What the import history shows for a row with no transaction to preview: a
// tap that was skipped, is still waiting, or whose transaction was deleted.
// A deleted one can be imported again: nothing else brings it back.
export function ImportedTapDialog({ row, cardName, cardLabel, onCreate, onClose }: ImportedTapDialogProps) {
  const { t } = useTranslation()
  const online = useOnline()
  const reimport = useReimportImportLink()
  if (!row) {
    return null
  }
  const close = () => {
    reimport.reset()
    onClose()
  }
  const runReimport = () => {
    reimport.mutate(row.id, {
      onSuccess: (res) => {
        const outcome = {
          matched: t('imports.history.reimported.matched'),
          queued: t('imports.history.reimported.queued'),
          skipped: t('imports.history.reimported.skipped'),
        }[res.status as string]
        toast.success(outcome ?? t('imports.history.reimported.created'))
        close()
      },
    })
  }
  const status = rowStatus(row)
  // the phone's wall clock, shown as is (see relativeTime.ts)
  const posted = parseDateTime(row.externalPostedAt)
  const when = `${posted.getFullYear()}-${pad(posted.getMonth() + 1)}-${pad(posted.getDate())} ${pad(posted.getHours())}:${pad(posted.getMinutes())}`
  const message =
    status === 'queued' ? t('imports.history.waiting')
      : status === 'deleted' ? `${t('imports.history.deleted')} ${t('imports.history.reimport_hint')}`
        : status === 'imported' ? t('imports.history.unavailable')
          : t('imports.history.not_added')
  const details: [string, string][] = [
    [t('transactions.modal.form.amount.label'), `${moneyFormat(row.externalAmount, null, { showCurrency: false, useNativePrecision: false })} ${row.externalCurrency}`],
    [cardLabel ?? t('imports.history.card'), cardName],
    [t('imports.history.date'), when],
  ]
  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && close()} title={row.externalPayee || row.externalTransactionId}>
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
        {reimport.isError ? <p role="alert" className="text-sm text-destructive">{apiErrorMessage(reimport.error)}</p> : null}
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={close}>{t('common.button.close.label')}</Button>
          {onCreate ? <Button type="button" onClick={onCreate}>{t('imports.history.create')}</Button> : null}
          {status === 'deleted' ? (
            <Button type="button" onClick={runReimport} disabled={!online || reimport.isPending}>{t('imports.history.reimport')}</Button>
          ) : null}
        </div>
      </div>
    </ResponsiveDialog>
  )
}
