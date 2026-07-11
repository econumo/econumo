import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { ShareEntryList } from './ShareEntryList'
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

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={title}>
      <div className="flex flex-col gap-2">
        {entries.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('modules.connections.modals.share_access.list_empty')}</p>
        ) : (
          <>
            <p className="text-sm text-muted-foreground">{t('modules.connections.modals.share_access.tap_to_share')}</p>
            <ShareEntryList kind={kind} entries={entries} onPick={onPick} />
          </>
        )}
        <Button type="button" className="w-full h-11" onClick={onClose}>
          {t('elements.button.ok.label')}
        </Button>
      </div>
    </ResponsiveDialog>
  )
}
