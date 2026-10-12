import { useTranslation } from 'react-i18next'
import { RouterPage } from '@/app/router-pages'
import { Button } from '@/components/ui/button'
import { InboxSection, SkippedInboxSection } from '@/features/inbox/InboxPage'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { FailedImportsSection, ToReviewSection } from './QueueSections'
import { useImportQueue, useImportSources } from './queries'

export function AppleWalletInboxPage() {
  const { t } = useTranslation()
  const queue = useImportQueue()
  const sources = useImportSources()
  const wallet = (sources.data ?? []).find((s) => s.provider === 'apple-wallet')
  const mine = <T extends { sourceId: string }>(rows: T[] | undefined) => (wallet ? (rows ?? []).filter((r) => r.sourceId === wallet.id) : [])
  const queued = mine(queue.data?.queued)
  const failed = mine(queue.data?.failed)
  const skipped = mine(queue.data?.skipped)
  const error = queue.isError || sources.isError
  const loaded = (queue.data !== undefined || queue.isError) && (sources.data !== undefined || sources.isError)
  const empty = loaded && !error && queued.length + failed.length + skipped.length === 0

  return (
    <SettingsShell
      title={t('inbox.title')}
      backTo={RouterPage.SETTINGS_APPLE_WALLET}
      crumbs={[
        { label: t('settings.page.header_desktop'), to: RouterPage.SETTINGS },
        { label: t('imports.apple_wallet.header'), to: RouterPage.SETTINGS_APPLE_WALLET },
      ]}
    >
      <div className="mx-auto flex w-full max-w-xl flex-col gap-6">
        {empty ? <p className="px-1 text-sm text-muted-foreground">{t('inbox.empty')}</p> : null}
        {error ? (
          <div className="flex flex-col items-center gap-3 p-6 text-center">
            <p className="max-w-md text-sm text-muted-foreground">{t('common.app.error')}</p>
            <Button type="button" onClick={() => void Promise.all([queue.refetch(), sources.refetch()])}>{t('imports.queue.retry')}</Button>
          </div>
        ) : null}
        {queued.length > 0 ? (
          <InboxSection title={t('inbox.sections.to_review')}><ToReviewSection queued={queued} /></InboxSection>
        ) : null}
        {failed.length > 0 ? (
          <InboxSection title={t('inbox.sections.failed')}><FailedImportsSection failed={failed} /></InboxSection>
        ) : null}
        {skipped.length > 0 ? <SkippedInboxSection skipped={skipped} /> : null}
      </div>
    </SettingsShell>
  )
}
