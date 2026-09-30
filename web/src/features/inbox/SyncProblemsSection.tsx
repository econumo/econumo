import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import type { ImportSourceDto } from '@/api/dto/imports'
import { RouterPage } from '@/app/router-pages'
import { formatDateTime, parseDateTime } from '@/lib/datetime'

export function SyncProblemsSection({ sources }: { sources: ImportSourceDto[] }) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col gap-2">
      {sources.map((s) => (
        <Link
          key={s.id}
          to={s.provider === 'simplefin' ? RouterPage.SETTINGS_SIMPLEFIN : RouterPage.SETTINGS_APPLE_WALLET}
          className="flex flex-col gap-1 rounded-lg bg-econumo-card px-4 py-3 text-sm hover:bg-econumo-hover"
        >
          <span className="font-medium">{t(s.lastRunStatus === 'partial' ? 'inbox.sync.partial' : 'inbox.sync.failed', { source: s.name })}</span>
          {s.lastRunAt ? <span className="text-xs text-muted-foreground">{t('inbox.sync.when', { date: formatDateTime(parseDateTime(s.lastRunAt)) })}</span> : null}
          {s.lastRunError ? <span className="text-xs text-destructive">{s.lastRunError}</span> : null}
        </Link>
      ))}
    </div>
  )
}
