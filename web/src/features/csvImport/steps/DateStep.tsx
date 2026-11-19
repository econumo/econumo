import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { pluralPick } from '@/lib/plural'
import { ColumnChoices } from '../ColumnChoices'
import { columnTags } from '../columnLabels'
import { Example, Field, Segmented, SkippedNote } from '../controls'
import { detectDateOrder, parseDateCell, type DateOrder } from '../dates'
import { columnValues } from '../guess'
import type { StepBodyProps } from '../wizard'

function longDate(value: string, order: DateOrder, language: string): string | null {
  const parsed = parseDateCell(value, order)
  if (!parsed) return null
  const [y, m, d] = parsed.date.split('-').map(Number)
  return new Intl.DateTimeFormat(language, { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' }).format(
    new Date(Date.UTC(y, m - 1, d)),
  )
}

export function DateStep({ state, setAnswers }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const title = t('transactions.import_wizard.date.title')
  const { date } = state.answers
  const values = useMemo(() => (date.column === null ? [] : columnValues(state.file, date.column)), [state.file, date.column])
  const detected = useMemo(() => (date.column === null ? null : detectDateOrder(values, navigator.language)), [values, date.column])
  const unreadable = date.column === null ? 0 : state.rows.filter((row) => row.reason === 'no_date').length
  const ambiguous = detected?.ambiguous ? detected.example : null
  const setOrder = (order: DateOrder) => setAnswers('date', { date: { ...date, order } })

  const dmy = ambiguous ? (longDate(ambiguous, 'dmy', i18n.language) ?? ambiguous) : ''
  const mdy = ambiguous ? (longDate(ambiguous, 'mdy', i18n.language) ?? ambiguous) : ''
  const question = ambiguous ? t('transactions.import_wizard.date.question', { example: ambiguous, dmy, mdy }) : ''
  const sample = values[0] ?? null
  const read = sample === null ? null : longDate(sample, date.order, i18n.language)

  return (
    <div className="flex flex-col gap-6">
      <ColumnChoices
        profiles={state.profiles}
        label={title}
        selected={date.column}
        onSelect={(column) => setAnswers('date', { date: { ...date, column } })}
        fits={(type) => type === 'date' || type === 'text'}
        tags={columnTags(state.answers, t, i18n.language, ['date'])}
      />
      {ambiguous ? (
        <Field label={question}>
          <Segmented<DateOrder>
            label={question}
            value={date.order}
            options={[
              { value: 'dmy', label: dmy },
              { value: 'mdy', label: mdy },
            ]}
            onChange={setOrder}
          />
        </Field>
      ) : null}
      {sample !== null ? (
        <Example>
          {t('transactions.import_wizard.date.example', { value: sample })}{' '}
          <span className="text-foreground">{read ?? t('transactions.import_wizard.date.not_a_date')}</span>
        </Example>
      ) : null}
      {unreadable > 0 ? <SkippedNote>{pluralPick(t('transactions.import_wizard.date.skipped'), unreadable, i18n.language)}</SkippedNote> : null}
    </div>
  )
}
