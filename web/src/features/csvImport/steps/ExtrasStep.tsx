import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { Input } from '@/components/ui/input'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories, useLabels, useTags } from '@/features/classifications/queries'
import { useUserData } from '@/features/user/queries'
import { pluralPick } from '@/lib/plural'
import type { ParsedFile } from '../parse'
import type { WizardAnswers } from '../prepare'
import { unusedColumns, type StepBodyProps } from '../wizard'
import { StepTitle } from './StepTitle'

type Role = 'category' | 'payee' | 'tag' | 'labels'
const ROLES: Role[] = ['category', 'payee', 'tag', 'labels']
const SEPARATORS = [';', ',', '|', '\t']
const DEFAULT_SEPARATOR = ';'

const roleOf = (extras: WizardAnswers['extras'], column: number): Role | '' => ROLES.find((role) => extras[role] === column) ?? ''

// names are matched trimmed and case-insensitively, as the server matches them
const nameKey = (name: string) => name.trim().toLowerCase()

function distinctNew(file: ParsedFile, column: number | null, split: string | null, existing: Set<string>): number {
  if (column === null) return 0
  const found = new Set<string>()
  for (const row of file.rows) {
    const cell = row[column] ?? ''
    for (const part of split === null ? [cell] : cell.split(split)) {
      const key = nameKey(part)
      if (key !== '' && !existing.has(key)) found.add(key)
    }
  }
  return found.size
}

export function ExtrasStep({ state, setAnswers }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const { data: categories = [] } = useCategories()
  const { data: tags = [] } = useTags()
  const { data: labels = [] } = useLabels()
  const customId = useId()
  const groupName = useId()
  const { extras, account } = state.answers
  const [custom, setCustom] = useState(() => (SEPARATORS.includes(extras.labelsSeparator) ? '' : extras.labelsSeparator))
  const separator = extras.labelsSeparator || DEFAULT_SEPARATOR

  const ownerId = account?.kind === 'existing' ? (accounts.find((a) => a.id === account.accountId)?.owner.id ?? user?.id) : user?.id

  // Archived ones count too: the server finds them and does not create a second one.
  // Labels and tags share one name space.
  const existingCategories = useMemo(() => new Set(categories.filter((c) => c.ownerUserId === ownerId).map((c) => nameKey(c.name))), [categories, ownerId])
  const existingLabels = useMemo(
    () => new Set([...labels, ...tags].filter((item) => item.ownerUserId === ownerId).map((item) => nameKey(item.name))),
    [labels, tags, ownerId],
  )
  const newCategories = useMemo(() => distinctNew(state.file, extras.category, null, existingCategories), [state.file, extras.category, existingCategories])
  const newLabels = useMemo(() => distinctNew(state.file, extras.labels, separator, existingLabels), [state.file, extras.labels, separator, existingLabels])

  // a role belongs to one column: giving it to another column takes it from the first
  const assign = (column: number, role: Role | '') => {
    const next = { ...extras }
    for (const r of ROLES) if (next[r] === column) next[r] = null
    if (role !== '') next[role] = column
    setAnswers('extras', { extras: next })
  }

  const setSeparator = (value: string) => setAnswers('extras', { extras: { ...extras, labelsSeparator: value } })

  const columns = unusedColumns(state.answers, state.profiles)
  const title = t('transactions.import_wizard.extras.title')
  const presetClass =
    'cursor-pointer rounded-lg border px-3 py-1.5 text-sm transition-colors hover:bg-muted/50 has-[:checked]:border-primary has-[:checked]:bg-primary/5 has-[:focus-visible]:ring-3 has-[:focus-visible]:ring-ring/50'

  return (
    <div className="flex flex-col gap-3">
      <StepTitle>{title}</StepTitle>
      <ul className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3" aria-label={title}>
        {columns.map((p) => (
          <li key={p.index} className="flex min-w-0 flex-col gap-1 rounded-lg border p-3">
            <span className="truncate text-sm">{p.name}</span>
            {p.samples.map((sample, i) => (
              <span key={i} className="truncate text-xs text-muted-foreground">
                {sample}
              </span>
            ))}
            <select
              aria-label={t('transactions.import_wizard.extras.use_as', { name: p.name })}
              className="mt-1 rounded-md border bg-transparent px-2 py-1.5 text-sm"
              value={roleOf(extras, p.index)}
              onChange={(e) => assign(p.index, e.target.value as Role | '')}
            >
              <option value="">{t('transactions.import_wizard.extras.leave_out')}</option>
              {ROLES.map((role) => (
                <option key={role} value={role}>
                  {t(`transactions.import_wizard.extras.role.${role}`)}
                </option>
              ))}
            </select>
          </li>
        ))}
      </ul>

      {extras.labels !== null ? (
        <div className="flex flex-col gap-2">
          <span className="text-sm">{t('transactions.import_wizard.extras.separator_question')}</span>
          <div role="radiogroup" aria-label={t('transactions.import_wizard.extras.separator_question')} className="flex flex-wrap items-center gap-2">
            {SEPARATORS.map((value) => (
              <label key={value} className={presetClass}>
                <input
                  type="radio"
                  name={groupName}
                  className="sr-only"
                  checked={separator === value}
                  onChange={() => {
                    setCustom('')
                    setSeparator(value)
                  }}
                />
                {value === '\t' ? t('transactions.import_wizard.extras.tab') : value}
              </label>
            ))}
            <CardField label={t('transactions.import_wizard.extras.custom_separator')} htmlFor={customId}>
              <Input
                id={customId}
                className={cardFieldControlClass}
                value={custom}
                onChange={(e) => {
                  setCustom(e.target.value)
                  setSeparator(e.target.value)
                }}
              />
            </CardField>
          </div>
        </div>
      ) : null}

      {newCategories > 0 ? (
        <p role="status" className="text-xs text-muted-foreground">
          {pluralPick(t('transactions.import_wizard.extras.new_categories'), newCategories, i18n.language)}
        </p>
      ) : null}
      {newLabels > 0 ? (
        <p role="status" className="text-xs text-muted-foreground">
          {pluralPick(t('transactions.import_wizard.extras.new_labels'), newLabels, i18n.language)}
        </p>
      ) : null}
    </div>
  )
}
