import { useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { GripVertical, Plus, Trash2 } from 'lucide-react'
import { v7 as uuidv7 } from 'uuid'
import { Button } from '@/components/ui/button'
import { EntityIcon } from '@/components/EntityIcon'
import { IconPicker } from '@/components/IconPicker'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { SortableList, type SortableHandleProps } from '@/components/SortableList'
import type { Id } from '@/api/types'
import { useCategories } from '@/features/classifications/queries'
import { useUserData } from '@/features/user/queries'
import { useIsCompact } from '@/hooks/useIsCompact'
import { apiErrorMessage } from '@/lib/apiError'
import { defaultCategoryIcon } from '@/lib/icons'
import { cn } from '@/lib/utils'
import type { CategoryRow } from './draft'
import { DEFAULT_EXPENSE_CATEGORIES, DEFAULT_INCOME_CATEGORIES } from './defaults'
import { labelled, nameLengthValid, NAME_MAX, NAME_MIN } from './names'
import type { StepBodyProps } from './OnboardingPage'
import { StepLoadError } from './StepLoadError'
import { useDay0 } from './useDay0'

type CategoryType = CategoryRow['type']
type ListItem = { id: string; existing?: ExistingCategory; row?: CategoryRow }

export interface ExistingCategory {
  id: Id
  name: string
  type: CategoryType
  icon: string
}

interface CategoriesStepProps {
  // the user's own categories: listed read-only among the new rows
  existing: ExistingCategory[]
  // the user's own archived categories: never listed, but a default named like one is not offered again
  archived?: ExistingCategory[]
  // the rows last submitted, so a retry keeps their keys (and their operation ids)
  initialRows?: CategoryRow[]
  // the order last submitted after a reorder: existing category ids and row keys
  initialOrder?: string[]
  // order is passed only once the user has reordered, so an untouched list never re-sorts the server's
  onContinue: (rows: CategoryRow[], order?: string[], offered?: string[]) => void
  onSkip: () => void
  pending: boolean
  error?: ReactNode
}

const nameKey = (type: CategoryType, name: string) => `${type}:${name.trim().toLowerCase()}`

export const rowTile = 'grid size-8 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary'
export const rowInput =
  'min-w-0 flex-1 rounded-md bg-transparent px-1 py-1 outline-none placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring/50 aria-invalid:ring-2 aria-invalid:ring-destructive/40'

export function CategoriesStep({ existing, archived = [], initialRows, initialOrder, onContinue, onSkip, pending, error }: CategoriesStepProps) {
  const { t } = useTranslation()
  const isCompact = useIsCompact()
  const taken = new Set(existing.map((c) => nameKey(c.type, c.name)))
  const [stored, setRows] = useState<CategoryRow[]>(() => {
    // a draft from before rows could be removed kept unticked rows as `checked: false`
    const kept = (initialRows ?? []).filter((r) => (r as { checked?: boolean }).checked !== false)
    const seeded = kept.filter((r) => !taken.has(nameKey(r.type, labelled(r, t).name)))
    if (initialRows && initialRows.length > 0) return seeded
    // every offered row is created unless removed, so a user who already has categories is offered none
    if (existing.length > 0) return []
    const build = (list: typeof DEFAULT_EXPENSE_CATEGORIES, type: CategoryType): CategoryRow[] =>
      list.map((c) => {
        const labelKey = `onboarding.categories.${c.key}`
        return { key: uuidv7(), name: t(labelKey), labelKey, defaultKey: c.key, type, icon: c.icon }
      })
    const retired = new Set(archived.map((c) => nameKey(c.type, c.name)))
    return [...build(DEFAULT_EXPENSE_CATEGORIES, 'expense'), ...build(DEFAULT_INCOME_CATEGORIES, 'income')].filter(
      (r) => !taken.has(nameKey(r.type, r.name)) && !retired.has(nameKey(r.type, r.name)),
    )
  })
  // the defaults this form offered; a resumed draft no longer knows, so useDay0 keeps the first submit's list
  const [offered] = useState<string[]>(() =>
    initialRows && initialRows.length > 0 ? [] : stored.flatMap((r) => (r.defaultKey ? [r.defaultKey] : [])),
  )
  const rows = stored.map((r) => labelled(r, t))
  const [order, setOrder] = useState<string[]>(initialOrder ?? [])
  const [reordered, setReordered] = useState(Boolean(initialOrder && initialOrder.length > 0))
  const [drafts, setDrafts] = useState<Record<CategoryType, string>>({ expense: '', income: '' })
  const [pickingKey, setPickingKey] = useState<string | null>(null)
  // a name still being typed is short on its way to valid, so its error waits for blur
  const [nameTouched, setNameTouched] = useState<ReadonlySet<string>>(new Set())
  const touch = (key: string) => setNameTouched((prev) => new Set(prev).add(key))

  const shown = rows
  const rank = new Map(order.map((id, i) => [id, i]))
  const byRank = (a: ListItem, b: ListItem) => (rank.get(a.id) ?? order.length) - (rank.get(b.id) ?? order.length)
  const itemsOf = (type: CategoryType): ListItem[] =>
    [
      ...existing.filter((c) => c.type === type).map((c) => ({ id: c.id, existing: c })),
      ...shown.filter((r) => r.type === type).map((r) => ({ id: r.key, row: r })),
    ].sort(byRank)
  const fullOrder = (type?: CategoryType, ids?: string[]) =>
    (['expense', 'income'] as const).flatMap((g) => (g === type && ids ? ids : itemsOf(g).map((i) => i.id)))
  const reorder = (type: CategoryType, ids: string[]) => {
    setOrder(fullOrder(type, ids))
    setReordered(true)
  }
  const update = (key: string, patch: Partial<CategoryRow>) => setRows((prev) => prev.map((r) => (r.key === key ? { ...r, ...patch } : r)))
  const remove = (key: string) => setRows((prev) => prev.filter((r) => r.key !== key))

  const addDraft = (type: CategoryType) => {
    const name = drafts[type].trim()
    if (name === '') return
    const key = uuidv7()
    setRows((prev) => [...prev, { key, name, type, icon: defaultCategoryIcon }])
    touch(key)
    setDrafts((prev) => ({ ...prev, [type]: '' }))
  }

  // a blank row or a second row of the same name is left out
  const normalized = () => {
    const seen = new Set(taken)
    return rows.flatMap((r) => {
      const name = r.name.trim()
      const id = nameKey(r.type, name)
      if (name === '' || seen.has(id)) return []
      seen.add(id)
      return [{ ...r, name }]
    })
  }

  const invalidName = new Set(rows.filter((r) => r.name.trim() !== '' && !nameLengthValid(r.name)).map((r) => r.key))
  const canContinue = !pending && invalidName.size === 0 && existing.length + normalized().length > 0
  const submit = () => {
    if (!canContinue) return
    const ids = fullOrder()
    const at = new Map(ids.map((id, i) => [id, i]))
    const sorted = normalized().sort((a, b) => (at.get(a.key) ?? ids.length) - (at.get(b.key) ?? ids.length))
    onContinue(sorted, reordered ? ids : undefined, offered)
  }

  const editing = rows.find((r) => r.key === pickingKey)

  const handle = (name: string, props: SortableHandleProps) =>
    isCompact ? null : (
      <button
        type="button"
        aria-label={t('onboarding.categories_step.reorder_label', { name })}
        className="-ml-1 grid size-6 shrink-0 cursor-grab touch-none place-items-center rounded-md text-muted-foreground hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
        {...props.attributes}
        {...(props.listeners ?? {})}
      >
        <GripVertical className="size-4" />
      </button>
    )

  const renderItem = (item: ListItem, props: SortableHandleProps) => {
    if (item.existing) {
      const c = item.existing
      return (
        <div className="flex items-center gap-3 bg-card px-3 py-2" data-testid="category-row">
          {handle(c.name, props)}
          <span className={rowTile}>
            <EntityIcon name={c.icon} className="text-lg" />
          </span>
          <span className="min-w-0 flex-1 truncate px-1">{c.name}</span>
        </div>
      )
    }
    const row = item.row
    if (!row) return null
    const showInvalid = invalidName.has(row.key) && nameTouched.has(row.key)
    return (
      <div className="bg-card px-3 py-2" data-testid="category-row">
        <div className="flex items-center gap-3">
          {handle(row.name, props)}
          <button
            type="button"
            className={cn(rowTile, 'hover:bg-primary/15')}
            aria-label={t('onboarding.categories_step.icon_label')}
            onClick={() => setPickingKey(row.key)}
          >
            <EntityIcon name={row.icon} className="text-lg" />
          </button>
          <input
            className={rowInput}
            aria-label={t('onboarding.categories_step.name_label')}
            maxLength={64}
            value={row.name}
            aria-invalid={showInvalid}
            onChange={(e) => update(row.key, { name: e.target.value, labelKey: undefined })}
            onBlur={() => touch(row.key)}
          />
          <button
            type="button"
            aria-label={t('onboarding.categories_step.remove_label', { name: row.name })}
            className="-mr-1 grid size-8 shrink-0 place-items-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
            onClick={() => remove(row.key)}
          >
            <Trash2 className="size-4" />
          </button>
        </div>
        {showInvalid && (
          <p className={cn('mt-1 text-sm text-destructive', isCompact ? 'pl-11' : 'pl-[4.75rem]')}>
            {t('errors.category.name_length', { min: NAME_MIN, max: NAME_MAX })}
          </p>
        )}
      </div>
    )
  }

  const group = (type: CategoryType, title: string) => {
    const items = itemsOf(type)
    return (
      <section data-testid={`group-${type}`}>
        <h3 className="mt-4 mb-1.5 text-xs tracking-wider text-muted-foreground uppercase">{title}</h3>
        <div className="overflow-hidden rounded-xl border bg-card">
          <SortableList
            items={items}
            disabled={isCompact}
            className="divide-y divide-border/60"
            onReorder={(ids) => reorder(type, ids)}
            renderItem={renderItem}
          />
          <div className={cn('flex items-center gap-3 px-3 py-2', items.length > 0 && 'border-t border-border/60')}>
            <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-muted text-muted-foreground">
              <Plus className="size-4" />
            </span>
            <input
              className={rowInput}
              aria-label={`${title}: ${t('onboarding.categories_step.add_placeholder')}`}
              placeholder={t('onboarding.categories_step.add_placeholder')}
              maxLength={64}
              value={drafts[type]}
              onChange={(e) => setDrafts((prev) => ({ ...prev, [type]: e.target.value }))}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault()
                  addDraft(type)
                }
              }}
              onBlur={() => addDraft(type)}
            />
          </div>
        </div>
      </section>
    )
  }

  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-2xl font-normal">{t('onboarding.categories_step.heading')}</h2>
      <p className="text-sm text-muted-foreground">{t('onboarding.categories_step.subtext')}</p>

      {group('expense', t('onboarding.categories_step.expenses'))}
      {group('income', t('onboarding.categories_step.income'))}

      {error}
      <div className="mt-6 flex items-center justify-between">
        <Button type="button" variant="ghost" disabled={pending} onClick={onSkip}>
          {t('onboarding.categories_step.skip')}
        </Button>
        <Button type="button" disabled={!canContinue} onClick={submit}>
          {t('onboarding.categories_step.continue')}
        </Button>
      </div>

      <ResponsiveDialog
        open={editing !== undefined}
        onOpenChange={(o) => !o && setPickingKey(null)}
        title={t('onboarding.categories_step.icon_label')}
      >
        {editing && (
          <IconPicker
            value={editing.icon}
            onChange={(icon) => {
              update(editing.key, { icon })
              setPickingKey(null)
            }}
            aria-label={t('onboarding.categories_step.icon_label')}
          />
        )}
      </ResponsiveDialog>
    </section>
  )
}

export function CategoriesStepBody({ onDone, onSkip }: StepBodyProps) {
  const { data: user } = useUserData()
  const categories = useCategories()
  const day0 = useDay0()
  const [skipping, setSkipping] = useState(false)
  if (categories.isError) return <StepLoadError error={categories.error} onRetry={() => void categories.refetch()} />
  if (!day0.ready || categories.isPending) return null
  const own = (categories.data ?? []).filter((c) => c.ownerUserId === user?.id)
  return (
    <CategoriesStep
      existing={own.filter((c) => c.isArchived === 0)}
      archived={own.filter((c) => c.isArchived === 1)}
      initialRows={day0.draft?.categoryRows}
      initialOrder={day0.draft?.categoryOrder}
      pending={day0.pending || skipping}
      error={
        day0.error != null ? (
          <p role="alert" className="text-sm text-destructive">
            {apiErrorMessage(day0.error)}
          </p>
        ) : undefined
      }
      onContinue={(rows, order, offered) => {
        void day0
          .saveCategories(rows, order, offered)
          .then(() => onDone())
          .catch(() => undefined)
      }}
      onSkip={() => {
        setSkipping(true)
        void onSkip().finally(() => setSkipping(false))
      }}
    />
  )
}
