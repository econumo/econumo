import { useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Plus } from 'lucide-react'
import { v7 as uuidv7 } from 'uuid'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { EntityIcon } from '@/components/EntityIcon'
import { IconPicker } from '@/components/IconPicker'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import type { Id } from '@/api/types'
import { useCategories } from '@/features/classifications/queries'
import { useUserData } from '@/features/user/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { defaultCategoryIcon } from '@/lib/icons'
import { cn } from '@/lib/utils'
import type { CategoryRow } from './draft'
import { DEFAULT_EXPENSE_CATEGORIES, DEFAULT_INCOME_CATEGORIES } from './defaults'
import { nameLengthValid, NAME_MAX, NAME_MIN } from './names'
import type { StepBodyProps } from './OnboardingPage'
import { StepLoadError } from './StepLoadError'
import { useDay0 } from './useDay0'

type CategoryType = CategoryRow['type']

export interface ExistingCategory {
  id: Id
  name: string
  type: CategoryType
  icon: string
}

interface CategoriesStepProps {
  // the user's own categories: listed first, ticked and locked
  existing: ExistingCategory[]
  // the user's own archived categories: never listed, but a default named like one is not offered again
  archived?: ExistingCategory[]
  // the rows last submitted, so a retry keeps their keys (and their operation ids)
  initialRows?: CategoryRow[]
  // rows created by an earlier submit: the server list shows them, so they leave the form
  createdKeys?: ReadonlySet<string>
  onContinue: (rows: CategoryRow[]) => void
  onSkip: () => void
  pending: boolean
  error?: ReactNode
}

const nameKey = (type: CategoryType, name: string) => `${type}:${name.trim().toLowerCase()}`

export const rowTile = 'grid size-8 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary'
export const rowInput =
  'min-w-0 flex-1 rounded-md bg-transparent px-1 py-1 outline-none placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring/50 aria-invalid:ring-2 aria-invalid:ring-destructive/40'

export function CategoriesStep({ existing, archived = [], initialRows, createdKeys, onContinue, onSkip, pending, error }: CategoriesStepProps) {
  const { t } = useTranslation()
  const taken = new Set(existing.map((c) => nameKey(c.type, c.name)))
  const [rows, setRows] = useState<CategoryRow[]>(() => {
    const seeded = (initialRows ?? []).filter((r) => !taken.has(nameKey(r.type, r.name)))
    if (initialRows && initialRows.length > 0) return seeded
    const build = (list: typeof DEFAULT_EXPENSE_CATEGORIES, type: CategoryType): CategoryRow[] =>
      list.map((c) => ({ key: uuidv7(), name: t(`onboarding.categories.${c.key}`), type, icon: c.icon, checked: existing.length === 0 }))
    const retired = new Set(archived.map((c) => nameKey(c.type, c.name)))
    return [...build(DEFAULT_EXPENSE_CATEGORIES, 'expense'), ...build(DEFAULT_INCOME_CATEGORIES, 'income')].filter(
      (r) => !taken.has(nameKey(r.type, r.name)) && !retired.has(nameKey(r.type, r.name)),
    )
  })
  const [drafts, setDrafts] = useState<Record<CategoryType, string>>({ expense: '', income: '' })
  const [pickingKey, setPickingKey] = useState<string | null>(null)
  // a name still being typed is short on its way to valid, so its error waits for blur
  const [nameTouched, setNameTouched] = useState<ReadonlySet<string>>(new Set())
  const touch = (key: string) => setNameTouched((prev) => new Set(prev).add(key))

  const shown = rows.filter((r) => !createdKeys?.has(r.key))
  const update = (key: string, patch: Partial<CategoryRow>) => setRows((prev) => prev.map((r) => (r.key === key ? { ...r, ...patch } : r)))

  const addDraft = (type: CategoryType) => {
    const name = drafts[type].trim()
    if (name === '') return
    const key = uuidv7()
    setRows((prev) => [...prev, { key, name, type, icon: defaultCategoryIcon, checked: true }])
    touch(key)
    setDrafts((prev) => ({ ...prev, [type]: '' }))
  }

  // every row goes out, unticked ones unchecked: the draft keeps them so a revisit shows them as left
  const normalized = () => {
    const seen = new Set(taken)
    return rows.map((r) => {
      const name = r.name.trim()
      const id = nameKey(r.type, name)
      const checked = r.checked && name !== '' && !seen.has(id)
      if (checked) seen.add(id)
      return { ...r, name, checked }
    })
  }

  const invalidName = new Set(rows.filter((r) => r.checked && r.name.trim() !== '' && !nameLengthValid(r.name)).map((r) => r.key))
  const canContinue = !pending && invalidName.size === 0 && (existing.length > 0 || normalized().some((r) => r.checked))
  const submit = () => {
    if (canContinue) onContinue(normalized())
  }

  const editing = rows.find((r) => r.key === pickingKey)

  const group = (type: CategoryType, title: string) => (
    <section data-testid={`group-${type}`}>
      <h3 className="mt-4 mb-1.5 text-xs tracking-wider text-muted-foreground uppercase">{title}</h3>
      <ul className="divide-y divide-border/60 overflow-hidden rounded-xl border bg-card">
        {existing
          .filter((c) => c.type === type)
          .map((c) => (
            <li key={c.id} className="flex items-center gap-3 px-3 py-2" data-testid="category-row">
              <span className={rowTile}>
                <EntityIcon name={c.icon} className="text-lg" />
              </span>
              <span className="min-w-0 flex-1 truncate px-1">{c.name}</span>
              <Checkbox aria-label={c.name} className="size-5 rounded-md" checked disabled />
            </li>
          ))}
        {shown
          .filter((r) => r.type === type)
          .map((row) => {
            const showInvalid = invalidName.has(row.key) && nameTouched.has(row.key)
            return (
              <li key={row.key} className="px-3 py-2" data-testid="category-row">
                <div className="flex items-center gap-3">
                  <button
                    type="button"
                    className={cn(rowTile, 'hover:bg-primary/15')}
                    aria-label={t('onboarding.categories_step.icon_label')}
                    onClick={() => setPickingKey(row.key)}
                  >
                    <EntityIcon name={row.icon} className="text-lg" />
                  </button>
                  <input
                    className={cn(rowInput, !row.checked && 'text-muted-foreground')}
                    aria-label={t('onboarding.categories_step.name_label')}
                    maxLength={64}
                    value={row.name}
                    aria-invalid={showInvalid}
                    onChange={(e) => update(row.key, { name: e.target.value })}
                    onBlur={() => touch(row.key)}
                  />
                  <Checkbox
                    aria-label={row.name}
                    className="size-5 rounded-md"
                    checked={row.checked}
                    onCheckedChange={(v) => update(row.key, { checked: v === true })}
                  />
                </div>
                {showInvalid && (
                  <p className="mt-1 pl-11 text-sm text-destructive">{t('errors.category.name_length', { min: NAME_MIN, max: NAME_MAX })}</p>
                )}
              </li>
            )
          })}
        <li className="flex items-center gap-3 px-3 py-2">
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
        </li>
      </ul>
    </section>
  )

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
      createdKeys={new Set(Object.keys(day0.draft?.createdCategories ?? {}))}
      pending={day0.pending || skipping}
      error={
        day0.error != null ? (
          <p role="alert" className="text-sm text-destructive">
            {apiErrorMessage(day0.error)}
          </p>
        ) : undefined
      }
      onContinue={(rows) => {
        void day0
          .saveCategories(rows)
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
