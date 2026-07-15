import { ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { UserAvatar } from '@/components/UserAvatar'
import type { ShareEntry } from './shared'

interface ShareEntryListProps {
  kind: 'accounts' | 'budgets'
  entries: ShareEntry[]
  /** omitted = read-only rows (plain text, no buttons) */
  onPick?: (entry: ShareEntry) => void
}

export function ShareEntryList({ kind, entries, onPick }: ShareEntryListProps) {
  const { t } = useTranslation()

  const roleText = (entry: ShareEntry): string => {
    if (!entry.role) {
      return t(`modules.connections.${kind}.roles.no_access`)
    }
    const label = t(`modules.connections.${kind}.roles.${entry.role}`)
    if (entry.isAccepted === false) {
      return `${label} – ${t('modules.connections.modals.share_access.not_accepted')}`
    }
    return label
  }

  const row = (entry: ShareEntry) => (
    <>
      <UserAvatar avatar={entry.user.avatar} size="sm" />
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-sm">{entry.user.name}</span>
        <span className="text-xs text-muted-foreground">{roleText(entry)}</span>
      </span>
    </>
  )

  return (
    <ul className="flex flex-col">
      {entries.map((entry) => (
        <li key={entry.user.id}>
          {onPick ? (
            <button
              type="button"
              className="flex w-full items-center gap-3 rounded-md px-2 py-2 text-left hover:bg-accent"
              onClick={() => onPick(entry)}
            >
              {row(entry)}
              <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
            </button>
          ) : (
            <span className="flex w-full items-center gap-3 rounded-md px-2 py-2">{row(entry)}</span>
          )}
        </li>
      ))}
    </ul>
  )
}
