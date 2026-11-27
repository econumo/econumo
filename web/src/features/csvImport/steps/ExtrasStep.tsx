import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { useLabels, useTags } from '@/features/classifications/queries'
import { firstDistinct, hasGuess, nameKey, unusedColumns, type StepBodyProps } from '../wizard'
import { displaySamples, fromName } from '../columnLabels'
import { candidates } from '../candidates'
import { AnswerRow, ChoicesDone, Field, Segmented } from '../controls'
import { NAMES_SHOWN, NamesList } from '../NamesList'
import { namesInFile, useOwnerLists } from '../ownerLists'

type Role = 'tag' | 'labels'
const ROLES: Role[] = ['tag', 'labels']
const SEPARATORS = [';', ',', '|', '\t', '\n']
const DEFAULT_SEPARATOR = ';'
const SEPARATOR_NAMES: Record<string, string> = {
  '\t': 'transactions.import_wizard.extras.tab',
  '\n': 'transactions.import_wizard.extras.newline',
}

export function ExtrasStep({ state, setAnswers, choosing, change, closeChoices, keepChoices }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const { data: tags = [] } = useTags()
  const { data: labels = [] } = useLabels()
  const customId = useId()
  const { extras, account } = state.answers
  const [custom, setCustom] = useState(() => (SEPARATORS.includes(extras.labelsSeparator) ? '' : extras.labelsSeparator))
  const separator = extras.labelsSeparator || DEFAULT_SEPARATOR

  const { ownerId, readOnlyLists } = useOwnerLists(account)

  // Archived ones count too: the server finds them and does not create a second one.
  // Budget and reporting tags share one name space.
  const existingLabels = useMemo(
    () => new Set([...labels, ...tags].filter((item) => item.ownerUserId === ownerId).map((item) => nameKey(item.name))),
    [labels, tags, ownerId],
  )

  // the earlier steps' columns are never offered; a column belongs to one role, so giving it to another takes it from the first
  const offered = unusedColumns(state.answers, state.profiles)
  const likely = new Set(candidates('tag', state.profiles, state.answers))
  const choicesFor = (role: Role) => (likely.size === 0 ? offered : offered.filter((p) => likely.has(p.index) || p.index === extras[role]))
  const assign = (role: Role, column: number | null) => {
    const next = { ...extras }
    for (const r of ROLES) if (column !== null && next[r] === column) next[r] = null
    next[role] = column
    setAnswers('extras', { extras: next })
    keepChoices()
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
          {likely.size === 0 ? <p className="text-xs text-muted-foreground">{t('transactions.import_wizard.nothing_fits.tags')}</p> : null}
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
          <div className="flex items-start justify-end gap-3">
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

      {ROLES.filter((role) => extras[role] !== null).map((role) => (
        <NamesList
          key={role}
          heading={t(`transactions.import_wizard.extras.found.${role}`)}
          limit={NAMES_SHOWN}
          items={namesInFile(state.rows, role, role === 'labels' ? separator : null, existingLabels).map((entry) => ({
            key: entry.key,
            name: entry.name,
            count: entry.count,
            status: !entry.isNew
              ? t('transactions.import_wizard.extras.status_existing')
              : readOnlyLists
                ? t('transactions.import_wizard.status_left_out')
                : t('transactions.import_wizard.extras.status_new'),
          }))}
        />
      ))}
    </div>
  )
}
