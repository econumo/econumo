import { useState } from 'react'
import { AlertTriangle, CheckCircle2, XCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { useOnline } from '@/hooks/useOnline'
import { pluralPick } from '@/lib/plural'
import type { ImportResult } from '@/features/csvImport/upload'

const MAX_DISPLAY_ERRORS = 5
const MAX_DISPLAY_ROWS = 10
// rows the browser could not read carry a catalogue key instead of the server's text
const SKIPPED_KEY = 'transactions.import_wizard.skipped.'

interface ImportResultDialogProps {
  open: boolean
  result: ImportResult | null
  onClose: () => void
  /** re-sends the chunks that failed; offered while `result.failedChunks` is not empty */
  onRetry?: () => Promise<unknown>
}

export function ImportResultDialog({ open, result, onClose, onRetry }: ImportResultDialogProps) {
  const { t, i18n } = useTranslation()
  const online = useOnline()
  const [retrying, setRetrying] = useState(false)
  if (!result) return null

  const retry = () => {
    if (!onRetry) return
    setRetrying(true)
    void onRetry().finally(() => setRetrying(false))
  }

  const unconfirmed = result.unconfirmed ?? 0
  const outcome =
    result.failed === 0 && unconfirmed === 0
      ? { title: t('transactions.import_result.success_title'), Icon: CheckCircle2, tone: 'text-income' }
      : result.imported > 0 || unconfirmed > 0
        ? { title: t('transactions.import_result.partial_success_title'), Icon: AlertTriangle, tone: 'text-amber-700 dark:text-amber-300' }
        : { title: t('transactions.import_result.error_title'), Icon: XCircle, tone: 'text-destructive' }

  const formatRows = (rows: number[]): string => {
    if (rows.length === 0) return ''
    if (rows.length === 1) return `${t('transactions.import_result.row')} ${rows[0]}`
    const shown = rows.slice(0, MAX_DISPLAY_ROWS).join(', ')
    const extra = rows.length - MAX_DISPLAY_ROWS
    return extra > 0
      ? `${t('transactions.import_result.rows')} ${shown} +${extra} ${t('transactions.import_result.more')}`
      : `${t('transactions.import_result.rows')} ${shown}`
  }

  const shownErrors = result.errors.slice(0, MAX_DISPLAY_ERRORS)
  const extraErrors = result.errors.length - shownErrors.length

  return (
    // a retry in flight writes its result back into this dialog, so it cannot be closed under it
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && !retrying && onClose()} dismissible={!retrying} title={outcome.title}>
      <div className="flex flex-col gap-3">
        <outcome.Icon className={`mx-auto size-10 ${outcome.tone}`} aria-hidden />
        <div className="flex flex-col gap-1 text-center text-sm">
          {result.imported > 0 ? <p>{pluralPick(t('transactions.import_result.imported'), result.imported, i18n.language)}</p> : null}
          {result.failed > 0 ? <p>{pluralPick(t('transactions.import_result.failed'), result.failed, i18n.language)}</p> : null}
          {unconfirmed > 0 ? <p>{pluralPick(t('transactions.import_result.unconfirmed'), unconfirmed, i18n.language)}</p> : null}
        </div>

        {shownErrors.length > 0 ? (
          <div className="flex flex-col gap-1">
            <p className="text-sm font-medium">{t('transactions.import_result.errors_detail')}</p>
            <ul className="flex max-h-60 flex-col gap-1 overflow-y-auto">
              {shownErrors.map((error) => (
                <li key={error.message} className="text-xs text-muted-foreground">
                  <span className="text-foreground">{error.message.startsWith(SKIPPED_KEY) ? t(error.message) : error.message}</span>
                  {error.rows.length > 0 ? ` — ${formatRows(error.rows)}` : ''}
                </li>
              ))}
            </ul>
            {extraErrors > 0 ? (
              <p className="text-xs text-muted-foreground">{t('transactions.import_result.and_more', { count: extraErrors })}</p>
            ) : null}
          </div>
        ) : null}

        {onRetry && result.failedChunks.length > 0 ? (
          <Button type="button" variant="secondary" className="w-full h-11" disabled={!online || retrying} onClick={retry}>
            {t('transactions.import_result.retry')}
          </Button>
        ) : null}
        <Button type="button" className="w-full h-11" disabled={retrying} onClick={onClose}>
          {t('common.button.ok.label')}
        </Button>
      </div>
    </ResponsiveDialog>
  )
}
