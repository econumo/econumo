import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router'
import { RouterPage } from '@/app/router-pages'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { apiErrorMessage } from '@/lib/apiError'
import { ImportRunSummary } from './ImportRunSummary'
import { useImportRuns } from './queries'

export function ImportRunListPage() {
  const { t } = useTranslation()
  const [params] = useSearchParams()
  const sourceId = params.get('sourceId') ?? ''
  const runs = useImportRuns(sourceId)
  return (
    <SettingsShell title={t('imports.runs.header')} backTo={RouterPage.SETTINGS_DATA}>
      <div className="mx-auto flex w-full max-w-xl flex-col gap-2">
        {runs.isPending ? <p className="text-sm text-muted-foreground">{t('common.app.modal.loading.data_loading')}</p> : null}
        {runs.isError ? <p role="alert" className="text-sm text-destructive">{apiErrorMessage(runs.error)}</p> : null}
        {runs.data?.length === 0 ? <p className="rounded-lg bg-econumo-card px-4 py-3.5 text-sm text-muted-foreground">{t('imports.runs.empty')}</p> : null}
        {runs.data?.map((run) => <ImportRunSummary key={run.id} run={run} to={RouterPage.IMPORT_RUN.replace(':id', run.id)} />)}
      </div>
    </SettingsShell>
  )
}
