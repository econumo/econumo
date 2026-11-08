import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CommandEmpty, CommandGroup, CommandItem } from '@/components/ui/command'
import { EntityIcon } from '@/components/EntityIcon'
import { PickerSheet } from '@/components/PickerSheet'
import { rankByName } from '@/lib/search'
import { cn } from '@/lib/utils'
import type { Id } from '@/api/types'
import { useCategories, useLabels, usePayees, useTags } from '@/features/classifications/queries'
import { useUserData } from '@/features/user/queries'
import { categoryOptions } from '@/features/transactions/useTransactionForm'
import type { PaymentsFrom } from './useDebtForm'

export type PaymentsFromErrors = Partial<Record<keyof PaymentsFrom, string | undefined>>

export interface PaymentsFromEditorProps {
  value: PaymentsFrom
  onChange: (next: PaymentsFrom) => void
  /** converting a category row: the category is the row's and stays */
  categoryLocked?: boolean
  errors?: PaymentsFromErrors
}

interface Option {
  id: Id
  name: string
  icon?: string
}

/** options under a heading; a group with no heading lists its options bare */
interface OptionGroup {
  heading?: string
  options: Option[]
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
  onPick,
}: {
  title: string
  groups: OptionGroup[]
  value: string | null
  placeholder: string
  anyLabel?: string
  disabled?: boolean
  onPick: (id: string | null) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const selected = groups.flatMap((g) => g.options).find((o) => o.id === value)
  const shown = selected?.name ?? placeholder
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
                <CommandGroup key={group.heading} heading={group.heading} className="p-0">
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

/** the caller's live rows, plus the chosen one even when archived */
function ownLive<T extends { id: Id; ownerUserId: Id; isArchived: 0 | 1 }>(rows: T[], chosen: Id | null, ownerId: Id | undefined): T[] {
  return rows.filter((r) => r.id === chosen || (r.isArchived === 0 && (!ownerId || r.ownerUserId === ownerId)))
}

/**
 * Which payments count as the debt's, as a sentence of pickers: "Payments in
 * [category] · from [any payee] · tagged [any tag]". The tag slot holds a tag
 * or a reporting tag, never both. The category is required, the rest narrow
 * it down.
 */
export function PaymentsFromEditor({ value, onChange, categoryLocked = false, errors = {} }: PaymentsFromEditorProps) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const { data: categories = [] } = useCategories()
  const { data: payees = [] } = usePayees()
  const { data: tags = [] } = useTags()
  const { data: labels = [] } = useLabels()
  const ownerId = user?.id

  const categoryRows = categoryOptions(categories, 'expense', ownerId)
  if (value.categoryId && !categoryRows.some((c) => c.id === value.categoryId)) {
    const current = categories.find((c) => c.id === value.categoryId)
    if (current) {
      categoryRows.push(current)
    }
  }
  const set = (partial: Partial<PaymentsFrom>) => onChange({ ...value, ...partial })
  const error = errors.categoryId ?? errors.payeeId ?? errors.tagId ?? errors.labelId
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
    <div className="flex flex-col gap-1">
      <div role="group" aria-label={t('debts.payments_from.label')} className="flex flex-col gap-1.5 rounded-lg bg-econumo-card px-4 py-2.5">
        <span className="text-[11px] text-muted-foreground">{t('debts.payments_from.label')}</span>
        <p className="flex flex-wrap items-center gap-x-1 gap-y-1.5 text-sm">
          <span>{t('debts.payments_from.in')}</span>
          <SlotPicker
            title={t('debts.form.category.label')}
            groups={[{ options: categoryRows.map((c) => ({ id: c.id, name: c.name, icon: c.icon || 'pending' })) }]}
            value={value.categoryId}
            placeholder={t('debts.payments_from.choose_category')}
            disabled={categoryLocked}
            onPick={(categoryId) => set({ categoryId })}
          />
          <span aria-hidden className="text-muted-foreground">·</span>
          <span>{t('debts.payments_from.from')}</span>
          <SlotPicker
            title={t('debts.form.payee.label')}
            groups={[{ options: ownLive(payees, value.payeeId, ownerId).map((p) => ({ id: p.id, name: p.name })) }]}
            value={value.payeeId}
            placeholder={t('debts.payments_from.any_payee')}
            anyLabel={t('debts.payments_from.any_payee')}
            onPick={(payeeId) => set({ payeeId })}
          />
          <span aria-hidden className="text-muted-foreground">·</span>
          <span>{t('debts.payments_from.tagged')}</span>
          <SlotPicker
            title={t('debts.form.tag.label')}
            groups={[
              {
                heading: t('debts.payments_from.group_tags'),
                options: ownLive(tags, value.tagId, ownerId).map((tag) => ({ id: TAG + tag.id, name: tag.name, icon: tag.icon || 'sell' })),
              },
              {
                heading: t('debts.payments_from.group_labels'),
                options: ownLive(labels, value.labelId, ownerId).map((label) => ({ id: LABEL + label.id, name: label.name, icon: label.icon || 'label' })),
              },
            ]}
            value={tagValue}
            placeholder={t('debts.payments_from.any_tag')}
            anyLabel={t('debts.payments_from.any_tag')}
            onPick={pickTag}
          />
        </p>
        <p className="text-xs text-muted-foreground">{t('debts.payments_from.explainer')}</p>
      </div>
      {error ? <p className="px-1 text-sm text-destructive">{error}</p> : null}
    </div>
  )
}
