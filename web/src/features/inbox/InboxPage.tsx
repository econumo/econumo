import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { RouterPage } from '@/app/router-pages'
import { Button } from '@/components/ui/button'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { FailedImportsSection, SkippedSection, ToReviewSection } from '@/features/imports/QueueSections'
import { DueRecurringSection } from './DueRecurringSection'
import { SharingSection } from './SharingSection'
import { SyncProblemsSection } from './SyncProblemsSection'
import { useInbox } from './useInbox'

export function InboxSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-2">
      <h2 className="px-1 text-xs uppercase text-muted-foreground">{title}</h2>
      {children}
    </section>
  )
}

export function InboxPage() {
  const { t } = useTranslation()
  const inbox = useInbox()
  const [skippedOpen, setSkippedOpen] = useState(false)
  // React Query v5 keeps stale data on a failed background refetch, so an
  // empty cached queue + a failed refetch must not still read "All caught up".
  const empty = inbox.isLoaded && !inbox.importsError && inbox.count === 0 && inbox.skipped.length === 0

  return (
    <SettingsShell title={t('inbox.title')} backTo={RouterPage.HOME}>
      <div className="mx-auto flex w-full max-w-xl flex-col gap-6">
        {empty ? <p className="px-1 text-sm text-muted-foreground">{t('inbox.empty')}</p> : null}
        {inbox.dueRecurring.length > 0 ? (
          <InboxSection title={t('inbox.sections.due')}><DueRecurringSection items={inbox.dueRecurring} /></InboxSection>
        ) : null}
        {inbox.importsError ? (
          <div className="flex flex-col items-center gap-3 p-6 text-center">
            <p className="max-w-md text-sm text-muted-foreground">{t('common.app.error')}</p>
            <Button type="button" onClick={inbox.retryImports}>{t('imports.queue.retry')}</Button>
          </div>
        ) : null}
        {inbox.queued.length > 0 ? (
          <InboxSection title={t('inbox.sections.to_review')}><ToReviewSection queued={inbox.queued} /></InboxSection>
        ) : null}
        {inbox.syncProblems.length > 0 ? (
          <InboxSection title={t('inbox.sections.sync')}><SyncProblemsSection sources={inbox.syncProblems} /></InboxSection>
        ) : null}
        {inbox.failed.length > 0 ? (
          <InboxSection title={t('inbox.sections.failed')}><FailedImportsSection failed={inbox.failed} /></InboxSection>
        ) : null}
        {inbox.skipped.length > 0 ? (
          <section className="flex flex-col gap-2">
            <button type="button" aria-expanded={skippedOpen} onClick={() => setSkippedOpen((o) => !o)}
              className="flex items-center gap-1 px-1 text-left text-xs uppercase text-muted-foreground">
              {skippedOpen ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
              <span role="heading" aria-level={2}>{t('inbox.sections.skipped', { count: inbox.skipped.length })}</span>
            </button>
            {skippedOpen ? <SkippedSection skipped={inbox.skipped} /> : null}
          </section>
        ) : null}
        {inbox.invites.length > 0 ? (
          <InboxSection title={t('inbox.sections.sharing')}><SharingSection invites={inbox.invites} /></InboxSection>
        ) : null}
      </div>
    </SettingsShell>
  )
}
