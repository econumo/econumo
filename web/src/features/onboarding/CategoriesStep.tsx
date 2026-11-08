import { useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { v7 as uuidv7 } from 'uuid'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { EntityIcon } from '@/components/EntityIcon'
import { IconPicker } from '@/components/IconPicker'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { defaultCategoryIcon } from '@/lib/icons'
import type { CategoryRow } from './draft'
import { DEFAULT_EXPENSE_CATEGORIES, DEFAULT_INCOME_CATEGORIES } from './defaults'
import { nameLengthValid, NAME_MAX, NAME_MIN } from './names'

type CategoryType = CategoryRow['type']

interface CategoriesStepProps {
  initialRows?: CategoryRow[]
  createdKeys?: ReadonlySet<string>
  onContinue: (rows: CategoryRow[]) => void
  onSkip: () => void
  pending: boolean
  error?: ReactNode
}

export function CategoriesStep({ initialRows, createdKeys, onContinue, onSkip, pending, error }: CategoriesStepProps) {
  const { t } = useTranslation()
  const [rows, setRows] = useState<CategoryRow[]>(() => {
    if (initialRows && initialRows.length > 0) return initialRows
    const build = (list: typeof DEFAULT_EXPENSE_CATEGORIES, type: CategoryType): CategoryRow[] =>
      list.map((c) => ({ key: uuidv7(), name: t(`onboarding.categories.${c.key}`), type, icon: c.icon, checked: true }))
    return [...build(DEFAULT_EXPENSE_CATEGORIES, 'expense'), ...build(DEFAULT_INCOME_CATEGORIES, 'income')]
  })
  const [drafts, setDrafts] = useState<Record<CategoryType, string>>({ expense: '', income: '' })
  const [pickingKey, setPickingKey] = useState<string | null>(null)
  // a name still being typed is short on its way to valid, so its error waits for blur
  const [nameTouched, setNameTouched] = useState<ReadonlySet<string>>(new Set())
  const touch = (key: string) => setNameTouched((prev) => new Set(prev).add(key))

  const isCreated = (row: CategoryRow) => createdKeys?.has(row.key) ?? false
  const update = (key: string, patch: Partial<CategoryRow>) => setRows((prev) => prev.map((r) => (r.key === key ? { ...r, ...patch } : r)))

  const addDraft = (type: CategoryType) => {
    const name = drafts[type].trim()
    if (name === '') return
    const key = uuidv7()
    setRows((prev) => [...prev, { key, name, type, icon: defaultCategoryIcon, checked: true }])
    touch(key)
    setDrafts((prev) => ({ ...prev, [type]: '' }))
  }

  const kept = () => {
    const seen = new Set<string>()
    const chosen = new Set<string>()
    // created rows go first so a duplicate-named new row can never displace one already on the server
    for (const r of [...rows.filter(isCreated), ...rows.filter((x) => !isCreated(x))]) {
      const name = r.name.trim()
      const id = `${r.type}:${name.toLowerCase()}`
      if ((!r.checked && !isCreated(r)) || name === '' || seen.has(id)) continue
      seen.add(id)
      chosen.add(r.key)
    }
    return rows.filter((r) => chosen.has(r.key)).map((r) => ({ ...r, name: r.name.trim(), checked: true }))
  }

  const invalidName = new Set(
    rows.filter((r) => r.checked && !isCreated(r) && r.name.trim() !== '' && !nameLengthValid(r.name)).map((r) => r.key),
  )
  const canContinue = !pending && invalidName.size === 0 && kept().length > 0
  const submit = () => {
    if (canContinue) onContinue(kept())
  }

  const editing = rows.find((r) => r.key === pickingKey)

  const group = (type: CategoryType, title: string) => (
    <section className="space-y-2" data-testid={`group-${type}`}>
      <h2 className="text-muted-foreground">{title}</h2>
      <ul className="space-y-2">
        {rows
          .filter((r) => r.type === type)
          .map((row) => {
            const created = isCreated(row)
            const showInvalid = invalidName.has(row.key) && nameTouched.has(row.key)
            return (
              <li key={row.key} className="rounded-xl border bg-card p-2" data-testid="category-row">
                <div className="flex items-center gap-2">
                  <Checkbox
                    aria-label={row.name}
                    checked={row.checked || created}
                    disabled={created}
                    onCheckedChange={(v) => update(row.key, { checked: v === true })}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label={t('onboarding.categories_step.icon_label')}
                    disabled={created}
                    onClick={() => setPickingKey(row.key)}
                  >
                    <EntityIcon name={row.icon} className="text-2xl text-muted-foreground" />
                  </Button>
                  <Input
                    aria-label={t('onboarding.categories_step.name_label')}
                    maxLength={64}
                    value={row.name}
                    readOnly={created}
                    aria-invalid={showInvalid}
                    onChange={(e) => update(row.key, { name: e.target.value })}
                    onBlur={() => touch(row.key)}
                  />
                </div>
                {showInvalid && (
                  <p className="mt-1 text-sm text-destructive">{t('errors.category.name_length', { min: NAME_MIN, max: NAME_MAX })}</p>
                )}
              </li>
            )
          })}
        <li className="rounded-xl border border-dashed p-2">
          <Input
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
    <div className="space-y-6">
      <header className="space-y-1">
        <h1 className="text-2xl">{t('onboarding.categories_step.heading')}</h1>
        <p className="text-muted-foreground">{t('onboarding.categories_step.subtext')}</p>
      </header>

      {group('expense', t('onboarding.categories_step.expenses'))}
      {group('income', t('onboarding.categories_step.income'))}

      <div className="flex flex-col items-center gap-3">
        {error}
        <Button type="button" className="w-full" disabled={!canContinue} onClick={submit}>
          {t('onboarding.categories_step.continue')}
        </Button>
        <Button type="button" variant="link" disabled={pending} onClick={onSkip}>
          {t('onboarding.categories_step.skip')}
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
    </div>
  )
}
