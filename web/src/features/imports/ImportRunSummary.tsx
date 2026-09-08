import { useTranslation } from 'react-i18next'
import type { ImportRunDto } from '@/api/dto/imports'
import { formatDateTime, parseDateTime } from '@/lib/datetime'
import { cn } from '@/lib/utils'

// Matches the tone palette ImportResultDialog already uses for the same
// import-outcome concept (success/partial/failure) — no dedicated
// "econumo-green"/"econumo-yellow" tokens exist in index.css.
const STATUS_CLASS: Record<ImportRunDto['status'], string> = {
  running: 'text-muted-foreground',
  completed: 'text-green-600',
  partial: 'text-amber-600',
  failed: 'text-destructive',
}

export function ImportRunSummary({ run, accountName }: { run: ImportRunDto; accountName?: (externalAccountId: string) => string }) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col gap-1 rounded-lg bg-econumo-card px-4 py-3.5 text-sm">
      <div className="flex items-center justify-between gap-2">
        <span className={cn('font-medium', STATUS_CLASS[run.status])}>{t(`imports.runs.status.${run.status}`)}</span>
        <span className="text-xs text-muted-foreground">{formatDateTime(parseDateTime(run.startedAt))}</span>
      </div>
      <div className="text-xs text-muted-foreground">
        {t('imports.runs.counts', {
          imported: run.importedCount, matched: run.matchedCount, updated: run.amountsUpdatedCount,
          queued: run.queuedCount, skipped: run.skippedCount, failed: run.failedCount,
        })}
      </div>
      {run.errors.map((e, i) => (
        <div key={i} className="text-xs text-destructive">
          {e.externalAccountId ? `${accountName?.(e.externalAccountId) || e.externalAccountId}: ` : ''}{e.message}
        </div>
      ))}
    </div>
  )
}
