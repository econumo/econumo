import { useTranslation } from 'react-i18next'
import { OfflineUnavailable } from '@/components/OfflineUnavailable'
import { useUnavailableOffline } from '@/lib/offlineQuery'
import { RouterPage } from '@/app/router-pages'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { apiErrorMessage } from '@/lib/apiError'
import { ImportRunSummary } from './ImportRunSummary'
import { useImportRuns, useImportSources } from './queries'

// A pull provider's history is its syncs: each run lists the rows it wrote.
export function SimpleFINHistoryPage() {
  const { t } = useTranslation()
  const sourcesQuery = useImportSources()
  const { data: sources } = sourcesQuery
  const source = sources?.find((s) => s.provider === 'simplefin')
  const runs = useImportRuns(source?.id)
  const items = source ? runs.data : []
  const offline = useUnavailableOffline(sourcesQuery, ...(source ? [runs] : []))
  return (
    <SettingsShell
      title={t('imports.runs.header')}
      backTo={RouterPage.SETTINGS_SIMPLEFIN}
      crumbs={[
        { label: t('settings.page.header_desktop'), to: RouterPage.SETTINGS },
        { label: t('imports.simplefin.title'), to: RouterPage.SETTINGS_SIMPLEFIN },
      ]}
    >
      <div className="mx-auto flex w-full max-w-xl flex-col gap-2">
        {offline ? <OfflineUnavailable /> : null}
        {source && runs.isPending && !offline ? <p className="text-sm text-muted-foreground">{t('common.app.modal.loading.data_loading')}</p> : null}
        {source && runs.isError && !offline ? <p role="alert" className="text-sm text-destructive">{apiErrorMessage(runs.error)}</p> : null}
        {sources && items?.length === 0 ? <p className="rounded-lg bg-econumo-card px-4 py-3.5 text-sm text-muted-foreground">{t('imports.runs.empty')}</p> : null}
        {items?.map((run) => <ImportRunSummary key={run.id} run={run} to={RouterPage.SETTINGS_SIMPLEFIN_RUN.replace(':id', run.id)} />)}
      </div>
    </SettingsShell>
  )
}
