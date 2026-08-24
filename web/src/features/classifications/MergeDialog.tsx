import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { EntityIcon } from '@/components/EntityIcon'
import { InfoBox } from '@/components/InfoBox'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { fuzzyMatch } from '@/lib/fuzzy'
import type { ClassificationItem } from './ClassificationList'

interface MergeDialogProps<T extends ClassificationItem> {
  open: boolean
  /** the item being absorbed and deleted */
  source: T | null
  /** eligible targets; the caller filters to own items of a compatible kind */
  candidates: T[]
  /** what the merge moves, e.g. "All transactions and recurring transactions…" */
  warning: string
  /** extra note shown above the picker (categories mention budget envelopes) */
  info?: string
  showIcon?: boolean
  onClose: () => void
  onConfirm: (targetId: string) => void
}

/**
 * Picks the surviving item for a merge. The acted-on row is the one that
 * disappears, so the copy names both sides explicitly and the confirm stays
 * disabled until a target is chosen — there is no undo.
 */
export function MergeDialog<T extends ClassificationItem>({
  open,
  source,
  candidates,
  warning,
  info,
  showIcon,
  onClose,
  onConfirm,
}: MergeDialogProps<T>) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const [targetId, setTargetId] = useState<string | null>(null)

  // a reopened dialog must not inherit the previous merge's choice
  useEffect(() => {
    if (open) {
      setQuery('')
      setTargetId(null)
    }
  }, [open, source?.id])

  const eligible = candidates.filter((item) => item.id !== source?.id)
  const trimmed = query.trim()
  const visible = trimmed ? eligible.filter((item) => fuzzyMatch(item.name, trimmed)) : eligible

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={t('classifications.common.merge.title', { name: source?.name ?? '' })}
    >
      <div className="flex flex-col gap-3">
        {info ? <InfoBox>{info}</InfoBox> : null}
        <Input
          aria-label={t('common.list.search')}
          placeholder={t('common.list.search')}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <div className="flex max-h-64 flex-col overflow-y-auto" role="listbox">
          {visible.length === 0 ? (
            <p className="px-1 py-2 text-sm text-muted-foreground">{t('common.list.search_empty')}</p>
          ) : (
            visible.map((item) => (
              <button
                key={item.id}
                type="button"
                role="option"
                aria-selected={targetId === item.id}
                className={`flex items-center gap-2 rounded-md px-2 py-2 text-left text-sm hover:bg-accent ${
                  targetId === item.id ? 'bg-accent' : ''
                }`}
                onClick={() => setTargetId(item.id)}
              >
                {showIcon ? <EntityIcon name={item.icon} className="text-base text-muted-foreground" /> : null}
                <span className="truncate">{item.name}</span>
              </button>
            ))
          )}
        </div>
        <p className="text-sm text-muted-foreground">{warning}</p>
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={targetId === null}
            onClick={() => targetId !== null && onConfirm(targetId)}
          >
            {t('classifications.common.merge.confirm')}
          </Button>
        </div>
      </div>
    </ResponsiveDialog>
  )
}
