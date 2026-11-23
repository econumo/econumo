import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { pluralPick } from '@/lib/plural'
import { formatDay } from '../accountView'
import { ColumnChoices } from '../ColumnChoices'
import { columnTags, fromName } from '../columnLabels'
import { Answer, AnswerActions, Field, Segmented, SkippedNote } from '../controls'
import { detectDateOrder, parseDateCell, type DateOrder } from '../dates'
import { columnValues } from '../guess'
import { firstDistinct, hasGuess, type StepBodyProps } from '../wizard'

// "3 November 2026", or "3 November" without the year for the question itself
function longDate(value: string, order: DateOrder, language: string, withYear = true): string | null {
  const parsed = parseDateCell(value, order)
  if (!parsed) return null
  const [y, m, d] = parsed.date.split('-').map(Number)
  return new Intl.DateTimeFormat(language, { day: 'numeric', month: 'long', ...(withYear ? { year: 'numeric' } : {}), timeZone: 'UTC' }).format(
    new Date(Date.UTC(y, m - 1, d)),
  )
}

export function DateStep({ state, setAnswers, choosing, choose }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const { date } = state.answers
  const values = useMemo(() => (date.column === null ? [] : columnValues(state.file, date.column)), [state.file, date.column])
  const detected = useMemo(() => (date.column === null ? null : detectDateOrder(values, navigator.language)), [values, date.column])
  const unreadable = date.column === null ? 0 : state.rows.filter((row) => row.reason === 'no_date').length
  const ambiguous = detected?.ambiguous ? detected.example : null
  const setOrder = (order: DateOrder) => setAnswers('date', { date: { ...date, order } })

  const dmy = ambiguous ? (longDate(ambiguous, 'dmy', i18n.language) ?? ambiguous) : ''
  const mdy = ambiguous ? (longDate(ambiguous, 'mdy', i18n.language) ?? ambiguous) : ''
  const question = ambiguous
    ? t('transactions.import_wizard.date.question', {
        example: ambiguous,
        dmy: longDate(ambiguous, 'dmy', i18n.language, false) ?? ambiguous,
        mdy: longDate(ambiguous, 'mdy', i18n.language, false) ?? ambiguous,
      })
    : ''
  const guessed = hasGuess('date', state)
  const open = choosing || !guessed
  const days = firstDistinct(state.rows, (row) => row.date)

  return (
    <div className="flex flex-col gap-6">
      {guessed ? (
        <Answer from={date.column === null ? null : fromName(state.profiles[date.column], state.profiles, t)}>
          {days.values.map((day) => formatDay(day, i18n.language)).join(' · ')}
          {days.more ? ' …' : ''}
        </Answer>
      ) : null}
      {open ? (
        <ColumnChoices
          profiles={state.profiles}
          label={t('transactions.import_wizard.date.choose')}
          selected={date.column}
          onSelect={(column) => setAnswers('date', { date: { ...date, column } })}
          fits={(type) => type === 'date' || type === 'text'}
          tags={columnTags(state.answers, t, i18n.language, ['date'])}
        />
      ) : null}
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
      {unreadable > 0 ? <SkippedNote>{pluralPick(t('transactions.import_wizard.date.skipped'), unreadable, i18n.language)}</SkippedNote> : null}
      {open ? null : <AnswerActions onChoose={choose} />}
    </div>
  )
}
