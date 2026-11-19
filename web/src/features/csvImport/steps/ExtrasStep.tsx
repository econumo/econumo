import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories, useLabels, useTags } from '@/features/classifications/queries'
import { useUserData } from '@/features/user/queries'
import { pluralPick } from '@/lib/plural'
import type { ParsedFile } from '../parse'
import { unusedColumns, type StepBodyProps } from '../wizard'
import { columnName } from '../columnLabels'
import { Field, Segmented } from '../controls'

type Role = 'category' | 'payee' | 'tag' | 'labels'
const ROLES: Role[] = ['payee', 'category', 'tag', 'labels']
const SEPARATORS = [';', ',', '|', '\t']
const DEFAULT_SEPARATOR = ';'

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

  // the main steps' columns are never offered; a column belongs to one role, so giving it to another takes it from the first
  const offered = unusedColumns(state.answers, state.profiles)
  const assign = (role: Role, column: number | null) => {
    const next = { ...extras }
    for (const r of ROLES) if (column !== null && next[r] === column) next[r] = null
    next[role] = column
    setAnswers('extras', { extras: next })
  }

  const setSeparator = (value: string) => setAnswers('extras', { extras: { ...extras, labelsSeparator: value } })
  const separatorQuestion = t('transactions.import_wizard.extras.separator_question')

  return (
    <div className="flex flex-col gap-5">
      <div className="divide-y divide-border rounded-xl border border-border">
        {ROLES.map((role) => {
          const label = t(`transactions.import_wizard.extras.role.${role}`)
          return (
            <div key={role} className="flex flex-col gap-2 p-4 sm:flex-row sm:items-center">
              <div className="min-w-0 flex-1">
                <div className="text-sm">{label}</div>
                <div className="text-xs text-muted-foreground">{t(`transactions.import_wizard.extras.role_hint.${role}`)}</div>
              </div>
              <NativeSelect
                aria-label={label}
                className="w-full sm:w-64"
                value={extras[role] === null ? '' : String(extras[role])}
                onChange={(e) => assign(role, e.target.value === '' ? null : Number(e.target.value))}
              >
                <NativeSelectOption value="">{t('transactions.import_wizard.extras.leave_out')}</NativeSelectOption>
                {offered.map((p) => (
                  <NativeSelectOption key={p.index} value={String(p.index)}>
                    {p.samples.length > 0 ? `${columnName(p, t)} — ${p.samples.join(' · ')}` : columnName(p, t)}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
          )
        })}
      </div>

      {extras.labels !== null ? (
        <div className="flex flex-wrap items-end gap-3">
          <Field label={separatorQuestion}>
            <Segmented<string>
              label={separatorQuestion}
              value={SEPARATORS.includes(separator) && custom === '' ? separator : null}
              options={SEPARATORS.map((value) => ({ value, label: value === '\t' ? t('transactions.import_wizard.extras.tab') : value }))}
              onChange={(value) => {
                setCustom('')
                setSeparator(value)
              }}
            />
          </Field>
          <Field label={t('transactions.import_wizard.extras.custom_separator')} htmlFor={customId}>
            <Input
              id={customId}
              className="h-9 w-32"
              value={custom}
              onChange={(e) => {
                setCustom(e.target.value)
                setSeparator(e.target.value)
              }}
            />
          </Field>
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
