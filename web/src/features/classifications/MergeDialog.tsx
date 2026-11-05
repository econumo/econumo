import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { CommandEmpty, CommandItem } from '@/components/ui/command'
import { EntityIcon } from '@/components/EntityIcon'
import { InfoBox } from '@/components/InfoBox'
import { PickerSheet } from '@/components/PickerSheet'
import { dialogActionsClass } from '@/components/ResponsiveDialog'
import { rankByName } from '@/lib/search'
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
  const [targetId, setTargetId] = useState<string | null>(null)

  // a reopened dialog must not inherit the previous merge's choice
  useEffect(() => {
    if (open) {
      setTargetId(null)
    }
  }, [open, source?.id])

  const eligible = candidates.filter((item) => item.id !== source?.id)

  return (
    <PickerSheet
      open={open}
      onClose={onClose}
      title={t('classifications.common.merge.title', { name: source?.name ?? '' })}
      searchLabel={t('common.list.search')}
      // picking the target is a tap, and the choice still needs a confirm
      focusSearch={false}
      listClassName="max-h-64 max-md:max-h-none"
      header={info ? <InfoBox>{info}</InfoBox> : null}
      after={() => <p className="mt-3 text-sm text-muted-foreground">{warning}</p>}
      actions={
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="button" variant="destructive" disabled={targetId === null} onClick={() => targetId !== null && onConfirm(targetId)}>
            {t('classifications.common.merge.confirm')}
          </Button>
        </div>
      }
    >
      {(query) => (
        <>
          <CommandEmpty className="px-1 py-2 text-left text-muted-foreground">{t('common.list.search_empty')}</CommandEmpty>
          {rankByName(eligible, (item) => item.name, query).map((item) => (
            <CommandItem key={item.id} value={item.id} data-checked={targetId === item.id} onSelect={() => setTargetId(item.id)}>
              {showIcon ? <EntityIcon name={item.icon} className="text-base text-muted-foreground" /> : null}
              <span className="truncate">{item.name}</span>
            </CommandItem>
          ))}
        </>
      )}
    </PickerSheet>
  )
}
