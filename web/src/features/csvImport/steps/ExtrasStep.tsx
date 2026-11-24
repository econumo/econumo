import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories, useLabels, useTags } from '@/features/classifications/queries'
import { useUserData } from '@/features/user/queries'
import { pluralPick } from '@/lib/plural'
import type { CategoryType } from '@/api/dto/category'
import { isNegative } from '@/lib/decimal'
import type { PreparedRow } from '../prepare'
import { firstDistinct, hasGuess, unusedColumns, type StepBodyProps } from '../wizard'
import { displaySamples, fromName } from '../columnLabels'
import { candidates } from '../candidates'
import { AnswerRow, ChoicesDone, Field, Segmented } from '../controls'

type Role = 'category' | 'payee' | 'tag' | 'labels'
const ROLES: Role[] = ['payee', 'category', 'tag', 'labels']
const SEPARATORS = [';', ',', '|', '\t', '\n']
const DEFAULT_SEPARATOR = ';'
const SEPARATOR_NAMES: Record<string, string> = {
  '\t': 'transactions.import_wizard.extras.tab',
  '\n': 'transactions.import_wizard.extras.newline',
}

// names are matched trimmed and case-insensitively, as the server matches them
const nameKey = (name: string) => name.trim().toLowerCase()

// A category is found within its kind, as the server does: an income row (an amount that is
// not negative) naming the expense category "Food" gets an income "Food" of its own.
const categoryKey = (type: CategoryType, name: string) => `${type}:${nameKey(name)}`
const rowKind = (row: PreparedRow): CategoryType => (row.amount !== null && isNegative(row.amount) ? 'expense' : 'income')

// Only rows that will upload: a name found only on a skipped row is never created.
function distinctNew(rows: PreparedRow[], role: 'category' | 'labels', split: string | null, existing: Set<string>): number {
  const found = new Set<string>()
  for (const row of rows) {
    if (row.status !== 'ok') continue
    const cell = row[role]
    for (const part of split === null ? [cell] : cell.split(split)) {
      if (nameKey(part) === '') continue
      const key = role === 'category' ? categoryKey(rowKind(row), part) : nameKey(part)
      if (!existing.has(key)) found.add(key)
    }
  }
  return found.size
}

export function ExtrasStep({ state, setAnswers, choosing, change, closeChoices }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const { data: categories = [] } = useCategories()
  const { data: tags = [] } = useTags()
  const { data: labels = [] } = useLabels()
  const customId = useId()
  const { extras, account } = state.answers
  const [showAll, setShowAll] = useState(false)
  const [custom, setCustom] = useState(() => (SEPARATORS.includes(extras.labelsSeparator) ? '' : extras.labelsSeparator))
  const separator = extras.labelsSeparator || DEFAULT_SEPARATOR

  const target = account?.kind === 'existing' ? accounts.find((a) => a.id === account.accountId) : undefined
  const ownerId = target?.owner.id ?? user?.id
  // a member with a user grant imports into the owner's lists but cannot grow them: the server
  // leaves a name the owner does not have yet out of the row
  const readOnlyLists =
    target !== undefined && target.owner.id !== user?.id && target.sharedAccess.find((a) => a.user.id === user?.id)?.role !== 'admin'
  const anyDetail = ROLES.some((role) => extras[role] !== null)

  // Archived ones count too: the server finds them and does not create a second one.
  // Labels and tags share one name space.
  const existingCategories = useMemo(() => new Set(categories.filter((c) => c.ownerUserId === ownerId).map((c) => categoryKey(c.type, c.name))), [categories, ownerId])
  const existingLabels = useMemo(
    () => new Set([...labels, ...tags].filter((item) => item.ownerUserId === ownerId).map((item) => nameKey(item.name))),
    [labels, tags, ownerId],
  )
  const newCategories = useMemo(() => distinctNew(state.rows, 'category', null, existingCategories), [state.rows, existingCategories])
  const newLabels = useMemo(() => distinctNew(state.rows, 'labels', separator, existingLabels), [state.rows, separator, existingLabels])

  // the main steps' columns are never offered; a column belongs to one role, so giving it to another takes it from the first
  const offered = unusedColumns(state.answers, state.profiles)
  const likely = new Set(candidates('payee', state.profiles, state.answers))
  const hiddenCount = offered.filter((p) => !likely.has(p.index)).length
  const choicesFor = (role: Role) => (showAll || likely.size === 0 ? offered : offered.filter((p) => likely.has(p.index) || p.index === extras[role]))
  const assign = (role: Role, column: number | null) => {
    const next = { ...extras }
    for (const r of ROLES) if (column !== null && next[r] === column) next[r] = null
    next[role] = column
    setAnswers('extras', { extras: next })
  }

  const setSeparator = (value: string) => setAnswers('extras', { extras: { ...extras, labelsSeparator: value } })
  const separatorQuestion = t('transactions.import_wizard.extras.separator_question')
  const guessed = hasGuess('extras', state)
  const open = choosing || !guessed
  const option = (p: (typeof offered)[number]) => {
    const values = displaySamples(p, i18n.language)
    const shown = values.length > 0 ? values.join(' · ') : t('transactions.import_wizard.column_empty')
    const from = fromName(p, state.profiles, t)
    return from ? `${shown} — ${from}` : shown
  }

  return (
    <div className="flex flex-col gap-5">
      {open ? (
        <div className="flex flex-col gap-2">
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
                    {choicesFor(role).map((p) => (
                      <NativeSelectOption key={p.index} value={String(p.index)}>
                        {option(p)}
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                </div>
              )
            })}
          </div>
          <div className="flex items-start justify-between gap-3">
            {!showAll && likely.size > 0 && hiddenCount > 0 ? (
              <button type="button" className="text-sm text-muted-foreground underline-offset-2 hover:text-foreground hover:underline" onClick={() => setShowAll(true)}>
                {t('transactions.import_wizard.show_all')}
              </button>
            ) : (
              <span />
            )}
            {guessed ? <ChoicesDone onDone={closeChoices} /> : null}
          </div>
        </div>
      ) : (
        <AnswerRow onChange={change}>
          {t('transactions.import_wizard.extras.answer')}
          <ul aria-label={t('transactions.import_wizard.extras.answer')} className="mt-1.5 flex flex-col gap-1.5 text-sm">
            {ROLES.filter((role) => extras[role] !== null).map((role) => {
              const column = extras[role] as number
              const examples = firstDistinct(state.rows, (row) => row[role])
              const from = fromName(state.profiles[column], state.profiles, t)
              return (
                <li key={role} className="min-w-0 break-words">
                  <span className="text-muted-foreground">{t(`transactions.import_wizard.extras.role.${role}`)}: </span>
                  {examples.values.length > 0 ? examples.values.join(' · ') : t('transactions.import_wizard.column_empty')}
                  {examples.more ? ' …' : ''}
                  {from ? <span className="whitespace-nowrap text-muted-foreground/60"> ({from})</span> : null}
                </li>
              )
            })}
          </ul>
        </AnswerRow>
      )}

      {extras.labels !== null ? (
        <div className="flex flex-wrap items-end gap-3">
          <Field label={separatorQuestion}>
            <Segmented<string>
              label={separatorQuestion}
              value={SEPARATORS.includes(separator) && custom === '' ? separator : null}
              options={SEPARATORS.map((value) => ({ value, label: SEPARATOR_NAMES[value] ? t(SEPARATOR_NAMES[value]) : value }))}
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

      {readOnlyLists && anyDetail ? (
        <p role="status" className="text-xs text-muted-foreground">
          {t('transactions.import_wizard.extras.shared_owner_lists')}
        </p>
      ) : null}
      {!readOnlyLists && newCategories > 0 ? (
        <p role="status" className="text-xs text-muted-foreground">
          {pluralPick(t('transactions.import_wizard.extras.new_categories'), newCategories, i18n.language)}
        </p>
      ) : null}
      {!readOnlyLists && newLabels > 0 ? (
        <p role="status" className="text-xs text-muted-foreground">
          {pluralPick(t('transactions.import_wizard.extras.new_labels'), newLabels, i18n.language)}
        </p>
      ) : null}
    </div>
  )
}
