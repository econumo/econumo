import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { Inbox as InboxIcon } from 'lucide-react'
import { RouterPage } from '@/app/router-pages'
import { METRICS, trackEvent } from '@/lib/metrics'
import { formatInboxCount, useInbox } from './useInbox'

export function InboxButton({ variant }: { variant: 'row' | 'rail' }) {
  const { t } = useTranslation()
  const inbox = useInbox()
  const label = inbox.count > 0 ? t('inbox.open', { count: inbox.count }) : t('inbox.title')
  return (
    <Link
      to={RouterPage.INBOX}
      aria-label={label}
      title={label}
      onClick={() =>
        trackEvent(METRICS.INBOX_OPEN, {
          dueRecurring: inbox.dueRecurring.length,
          invites: inbox.invites.length,
          syncProblems: inbox.syncProblems.length,
          failed: inbox.failed.length,
          queued: inbox.queued.length,
        })
      }
      className={`relative grid shrink-0 place-items-center rounded-lg text-muted-foreground hover:bg-accent hover:text-foreground ${variant === 'rail' ? 'size-10' : 'size-9'}`}
    >
      <InboxIcon className="size-5" />
      {inbox.count > 0 ? (
        <span
          data-testid="inbox-badge"
          className="absolute -top-0.5 -right-0.5 min-w-4 rounded-full bg-primary px-1 text-center text-[10px] leading-4 text-primary-foreground"
        >
          {formatInboxCount(inbox.count)}
        </span>
      ) : null}
    </Link>
  )
}
