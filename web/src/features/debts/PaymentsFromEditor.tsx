import { useState } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { CommandEmpty, CommandGroup, CommandItem } from '@/components/ui/command'
import { EntityIcon } from '@/components/EntityIcon'
import { PickerSheet } from '@/components/PickerSheet'
import { rankByName } from '@/lib/search'
import { cn } from '@/lib/utils'
import type { Id } from '@/api/types'
import type { PaymentsFrom } from './rules'

export interface Option {
  id: Id
  name: string
  icon?: string
}

/** options under a heading; a group with no heading lists its options bare */
export interface OptionGroup {
  heading?: string
  options: Option[]
}

/** what each slot of the sentence offers: categories (grouped by owner), and
    the payees, tags and labels of the chosen category's owner */
export interface PaymentsFromChoices {
  categories: OptionGroup[]
  payees: Option[]
  tags: Option[]
  labels: Option[]
}

export interface PaymentsFromEditorProps {
  value: PaymentsFrom
  onChange: (next: PaymentsFrom) => void
  choices: PaymentsFromChoices
  /** converting a category row: the category is the row's and stays */
  categoryLocked?: boolean
  /** the sentence as text: the picks named, nothing to change */
  readOnly?: boolean
  /** shows a button that drops this sentence */
  onRemove?: () => void
}

const ANY = '__any__'

/** One word of the sentence: the chosen name, or what an empty slot means, and
    a picker sheet behind it. `anyLabel` makes the slot optional. Option ids
    are unique across the groups. */
function SlotPicker({
  title,
  groups,
  value,
  placeholder,
  anyLabel,
  disabled,
  readOnly,
  onPick,
}: {
  title: string
  groups: OptionGroup[]
  value: string | null
  placeholder: string
  anyLabel?: string
  disabled?: boolean
  readOnly?: boolean
  onPick: (id: string | null) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const selected = groups.flatMap((g) => g.options).find((o) => o.id === value)
  const shown = selected?.name ?? placeholder
  if (readOnly) {
    return (
      <span className={cn('inline-flex max-w-full items-center gap-1', !selected && 'text-muted-foreground')}>
        {selected?.icon ? <EntityIcon name={selected.icon} className="shrink-0 text-base text-muted-foreground" /> : null}
        <span className="truncate">{shown}</span>
      </span>
    )
  }
  const pick = (id: string | null) => {
    onPick(id)
    setOpen(false)
  }
  const item = (option: Option) => (
    <CommandItem key={option.id} value={option.id} data-checked={option.id === value} onSelect={() => pick(option.id)}>
      {option.icon ? <EntityIcon name={option.icon} className="text-base text-muted-foreground" /> : null}
      <span className="flex-1">{option.name}</span>
    </CommandItem>
  )

  return (
    <>
      <button
        type="button"
        aria-label={`${title}: ${shown}`}
        disabled={disabled}
        className={cn(
          'inline-flex max-w-full items-center gap-1 rounded-md bg-background px-2 py-0.5 text-sm text-econumo-link disabled:text-foreground',
          !disabled && 'hover:bg-accent',
          !selected && 'text-muted-foreground',
        )}
        onClick={() => setOpen(true)}
      >
        {selected?.icon ? <EntityIcon name={selected.icon} className="shrink-0 text-base" /> : null}
        <span className="truncate">{shown}</span>
      </button>
      <PickerSheet
        open={open}
        onClose={() => setOpen(false)}
        title={title}
        searchLabel={t('common.select.search_placeholder')}
        currentValue={value ?? (anyLabel ? ANY : undefined)}
      >
        {(search) => (
          <>
            <CommandEmpty />
            {anyLabel && search.trim() === '' ? (
              <CommandItem value={ANY} data-checked={value === null} onSelect={() => pick(null)}>
                <span className="flex-1 text-muted-foreground">{anyLabel}</span>
              </CommandItem>
            ) : null}
            {groups.map((group, i) => {
              const ranked = rankByName(group.options, (o) => o.name, search)
              if (ranked.length === 0) {
                return null
              }
              return group.heading ? (
                <CommandGroup key={i} heading={group.heading} className="p-0">
                  {ranked.map(item)}
                </CommandGroup>
              ) : (
                <div key={i}>{ranked.map(item)}</div>
              )
            })}
          </>
        )}
      </PickerSheet>
    </>
  )
}

const TAG = 'tag:'
const LABEL = 'label:'

/**
 * One rule of a debt as a sentence of pickers: "Payments in [category] · from
 * [any payee] · tagged [any tag]". The tag slot holds a tag or a reporting
 * tag, never both. The category is required, the rest narrow it down.
 */
export function PaymentsFromEditor({ value, onChange, choices, categoryLocked = false, readOnly = false, onRemove }: PaymentsFromEditorProps) {
  const { t } = useTranslation()
  const set = (partial: Partial<PaymentsFrom>) => onChange({ ...value, ...partial })
  const tagValue = value.tagId ? TAG + value.tagId : value.labelId ? LABEL + value.labelId : null
  const pickTag = (key: string | null) => {
    if (key?.startsWith(TAG)) {
      set({ tagId: key.slice(TAG.length), labelId: null })
    } else if (key?.startsWith(LABEL)) {
      set({ tagId: null, labelId: key.slice(LABEL.length) })
    } else {
      set({ tagId: null, labelId: null })
    }
  }

  return (
    <div className="flex items-start gap-2">
      {/* the spaces between the words are for its text; the flex gap spaces them on screen */}
      <p className="flex min-w-0 flex-1 flex-wrap items-center gap-x-1 gap-y-1.5 text-sm">
        <span>{t('debts.payments_from.in')}</span>
        {' '}
        <SlotPicker
          title={t('debts.form.category.label')}
          groups={choices.categories}
          value={value.categoryId}
          placeholder={t('debts.payments_from.choose_category')}
          readOnly={readOnly}
          disabled={categoryLocked}
          onPick={(categoryId) => set({ categoryId })}
        />
        {' '}
        <span aria-hidden className="text-muted-foreground">·</span>
        {' '}
        <span>{t('debts.payments_from.from')}</span>
        {' '}
        <SlotPicker
          title={t('debts.form.payee.label')}
          groups={[{ options: choices.payees }]}
          value={value.payeeId}
          placeholder={t('debts.payments_from.any_payee')}
          readOnly={readOnly}
          anyLabel={t('debts.payments_from.any_payee')}
          onPick={(payeeId) => set({ payeeId })}
        />
        {' '}
        <span aria-hidden className="text-muted-foreground">·</span>
        {' '}
        <span>{t('debts.payments_from.tagged')}</span>
        {' '}
        <SlotPicker
          title={t('debts.form.tag.label')}
          groups={[
            { heading: t('debts.payments_from.group_tags'), options: choices.tags.map((tag) => ({ ...tag, id: TAG + tag.id, icon: tag.icon || 'sell' })) },
            { heading: t('debts.payments_from.group_labels'), options: choices.labels.map((label) => ({ ...label, id: LABEL + label.id, icon: label.icon || 'label' })) },
          ]}
          value={tagValue}
          placeholder={t('debts.payments_from.any_tag')}
          readOnly={readOnly}
          anyLabel={t('debts.payments_from.any_tag')}
          onPick={pickTag}
        />
      </p>
      {onRemove && !readOnly ? (
        <Button type="button" variant="ghost" size="icon" className="-my-1 size-8 shrink-0" aria-label={t('debts.rules.remove')} title={t('debts.rules.remove')} onClick={onRemove}>
          <X className="size-4" />
        </Button>
      ) : null}
    </div>
  )
}
