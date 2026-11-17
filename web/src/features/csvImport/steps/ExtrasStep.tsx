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
import { usedColumns, type StepBodyProps } from '../wizard'
import { columnLabels } from '../columnLabels'
import { FileTable } from '../FileTable'
import { PillChoice } from './PillChoice'
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

  const title = t('transactions.import_wizard.extras.title')
  const hint = t('transactions.import_wizard.extras.hint')
  const locked = usedColumns(state.answers)
  const [picked, setPicked] = useState<number | null>(null)
  const pickedName = picked === null ? '' : state.file.header[picked]?.trim() || t('transactions.import_wizard.column_n', { n: picked + 1 })

  return (
    <div className="flex flex-col gap-3">
      <StepTitle>{title}</StepTitle>
      <p className="text-sm text-muted-foreground">{hint}</p>
      <FileTable
        file={state.file}
        label={hint}
        selected={picked}
        onSelect={setPicked}
        labels={columnLabels(state.answers, t)}
        locked={locked}
      />

      {picked !== null ? (
        <PillChoice<Role | ''>
          label={t('transactions.import_wizard.extras.use_as', { name: pickedName })}
          showLabel
          value={roleOf(extras, picked)}
          options={[
            { value: '', label: t('transactions.import_wizard.extras.leave_out') },
            ...ROLES.map((role) => ({ value: role, label: t(`transactions.import_wizard.extras.role.${role}`) })),
          ]}
          onChange={(role) => assign(picked, role)}
        />
      ) : null}

      {extras.labels !== null ? (
        <div className="flex flex-wrap items-center gap-2">
          <PillChoice<string>
            label={t('transactions.import_wizard.extras.separator_question')}
            showLabel
            value={SEPARATORS.includes(separator) && custom === '' ? separator : null}
            options={SEPARATORS.map((value) => ({ value, label: value === '\t' ? t('transactions.import_wizard.extras.tab') : value }))}
            onChange={(value) => {
              setCustom('')
              setSeparator(value)
            }}
          />
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
