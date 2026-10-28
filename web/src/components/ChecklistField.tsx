import type { ReactNode } from 'react'
import { ChevronRight, Lock } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { CommandEmpty, CommandGroup, CommandItem } from '@/components/ui/command'
import { EntityIcon } from '@/components/EntityIcon'
import { PickerSheet } from '@/components/PickerSheet'
import { dialogActionsClass } from '@/components/ResponsiveDialog'
import { rankByName } from '@/lib/search'
import type { Id } from '@/api/types'

// below this many items the checklist is scanned at a glance: no search
const SEARCH_THRESHOLD = 6

/** a form card naming what is picked; a tap opens the picker. The form stays
    the same size whether there are 3 choices or 30 */
export function PickerRow({
  label,
  aside,
  value,
  muted,
  disabled,
  testId,
  onOpen,
}: {
  label: string
  aside?: string
  value: string
  muted: boolean
  disabled?: boolean
  testId?: string
  onOpen: () => void
}) {
  return (
    <button
      type="button"
      className="flex w-full items-center justify-between gap-3 rounded-lg bg-econumo-card px-4 py-2.5 text-left hover:bg-econumo-hover disabled:opacity-60 disabled:hover:bg-econumo-card"
      data-testid={testId}
      disabled={disabled}
      onClick={onOpen}
    >
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex items-baseline justify-between gap-2 text-[11px] text-muted-foreground">
          <span>{label}</span>
          {aside ? <span>{aside}</span> : null}
        </span>
        <span className={`truncate text-sm ${muted ? 'text-muted-foreground' : ''}`}>{value}</span>
      </span>
      <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
    </button>
  )
}

interface ChecklistItem {
  id: Id
  name: string
  icon: string
}

export interface ChecklistGroup<T extends ChecklistItem> {
  heading?: string
  items: T[]
}

/** the multi-select picker sheet: tap rows to tick them, OK to close */
export function ChecklistSheet<T extends ChecklistItem>({
  open,
  title,
  description,
  groups,
  checked,
  disabled,
  itemLabel,
  onToggle,
  onClose,
  notes,
}: {
  open: boolean
  title: string
  description?: string
  groups: ChecklistGroup<T>[]
  checked: Set<Id>
  /** ticked and fixed: shown checked with a lock */
  disabled?: Set<Id>
  itemLabel?: (item: T) => string
  onToggle: (id: Id, on: boolean) => void
  onClose: () => void
  notes?: ReactNode
}) {
  const { t } = useTranslation()
  const total = groups.reduce((n, g) => n + g.items.length, 0)
  return (
    <PickerSheet
      open={open}
      onClose={onClose}
      title={title}
      description={description}
      searchLabel={t('accounts.page.toolbar.search')}
      searchable={total >= SEARCH_THRESHOLD}
      // a checklist is ticked by tapping: no keyboard over the list on open
      focusSearch={false}
      listClassName="max-h-96 max-md:max-h-none"
      actions={
        <div className={dialogActionsClass}>
          <Button type="button" className="col-span-2" onClick={onClose}>
            {t('common.button.ok.label')}
          </Button>
        </div>
      }
      after={() => (notes ? <div className="mt-3 flex flex-col gap-1 text-[11px] text-muted-foreground">{notes}</div> : null)}
    >
      {(search) => (
        <>
          <CommandEmpty>{t('common.list.list_empty')}</CommandEmpty>
          {groups.map((group) => {
            const shown = rankByName(group.items, (item) => item.name, search)
            if (shown.length === 0) {
              return null
            }
            return (
              <CommandGroup key={group.heading ?? ''} heading={group.heading}>
                {shown.map((item) => {
                  const on = checked.has(item.id)
                  const locked = !!disabled?.has(item.id)
                  return (
                    <CommandItem
                      key={item.id}
                      value={item.id}
                      data-checked={on}
                      // cmdk's aria-selected is its highlight; the tick is aria-checked
                      aria-checked={on}
                      disabled={locked}
                      // a locked member is still checked: it must not read as greyed-out/unchecked
                      className="data-[disabled=true]:opacity-100"
                      aria-label={itemLabel ? itemLabel(item) : item.name}
                      onSelect={() => onToggle(item.id, !on)}
                    >
                      <EntityIcon name={item.icon} className="text-lg text-muted-foreground" />
                      <span className="min-w-0 flex-1 truncate">{item.name}</span>
                      {locked ? <Lock className="size-3.5 text-muted-foreground" data-testid={`locked-${item.id}`} /> : null}
                    </CommandItem>
                  )
                })}
              </CommandGroup>
            )
          })}
        </>
      )}
    </PickerSheet>
  )
}
