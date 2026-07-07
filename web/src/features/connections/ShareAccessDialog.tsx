import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import type { ShareEntry } from './shared'

interface ShareAccessDialogProps {
  open: boolean
  title: string
  kind: 'accounts' | 'budgets'
  entries: ShareEntry[]
  onPick: (entry: ShareEntry) => void
  onClose: () => void
}

export function ShareAccessDialog({ open, title, kind, entries, onPick, onClose }: ShareAccessDialogProps) {
  const { t } = useTranslation()

  const roleText = (entry: ShareEntry): string => {
    if (!entry.role) {
      return t(`modules.connections.${kind}.roles.no_access`)
    }
    const label = t(`modules.connections.${kind}.roles.${entry.role}`)
    if (kind === 'budgets' && entry.isAccepted === false) {
      return `${label} – ${t('modules.connections.modals.share_access.not_accepted')}`
    }
    return label
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={title}>
      <div className="flex flex-col gap-2">
        {entries.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('modules.connections.modals.share_access.list_empty')}</p>
        ) : (
          <>
            <p className="text-sm text-muted-foreground">{t('modules.connections.modals.share_access.tap_to_share')}</p>
            <ul className="flex flex-col">
              {entries.map((entry) => (
                <li key={entry.user.id}>
                  <button
                    type="button"
                    className="flex w-full items-center gap-3 rounded-md px-2 py-2 text-left hover:bg-accent"
                    onClick={() => onPick(entry)}
                  >
                    <img src={`${entry.user.avatar}?s=50`} alt={entry.user.name} className="size-8 rounded-full" />
                    <span className="flex min-w-0 flex-1 flex-col">
                      <span className="truncate text-sm">{entry.user.name}</span>
                      <span className="text-xs text-muted-foreground">{roleText(entry)}</span>
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          </>
        )}
        <Button type="button" className="w-full h-11" onClick={onClose}>
          {t('elements.button.ok.label')}
        </Button>
      </div>
    </ResponsiveDialog>
  )
}
