import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { pluralPick } from '@/lib/plural'
import { ChoiceCards } from '../ChoiceCards'
import { columnMarks } from '../columnLabels'
import { FileTable } from '../FileTable'
import { detectDateOrder, parseDateCell, type DateOrder } from '../dates'
import { columnValues } from '../guess'
import { skippedRows, type StepBodyProps } from '../wizard'
import { SkippedNote } from './SkippedNote'
import { StepTitle } from './StepTitle'

function longDate(value: string, order: DateOrder, language: string): string {
  const parsed = parseDateCell(value, order)
  if (!parsed) return value
  const [y, m, d] = parsed.date.split('-').map(Number)
  return new Intl.DateTimeFormat(language, { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' }).format(
    new Date(Date.UTC(y, m - 1, d)),
  )
}

function OrderQuestion({ example, order, onChange }: { example: string; order: DateOrder; onChange: (order: DateOrder) => void }) {
  const { t, i18n } = useTranslation()
  const dmy = longDate(example, 'dmy', i18n.language)
  const mdy = longDate(example, 'mdy', i18n.language)
  const question = t('transactions.import_wizard.date.question', { example, dmy, mdy })
  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm">{question}</p>
      <ChoiceCards<DateOrder>
        label={question}
        value={order}
        options={[
          { value: 'dmy', label: dmy },
          { value: 'mdy', label: mdy },
        ]}
        onChange={onChange}
      />
    </div>
  )
}

export function DateStep({ state, setAnswers, goTo }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const title = t('transactions.import_wizard.date.title')
  const { date } = state.answers
  const detected = useMemo(
    () => (date.column === null ? null : detectDateOrder(columnValues(state.file, date.column), navigator.language)),
    [state.file, date.column],
  )
  const unreadable = date.column === null ? 0 : state.rows.filter((row) => row.reason === 'no_date').length
  const example = detected?.ambiguous ? detected.example : null

  return (
    <div className="flex flex-col gap-3">
      <StepTitle>{title}</StepTitle>
      <FileTable
        file={state.file}
        label={title}
        selected={date.column}
        onSelect={(column) => setAnswers('date', { date: { ...date, column } })}
        {...columnMarks(state.answers, t, 'date', ['date'])}
        onOpenStep={goTo}
        mutedRows={skippedRows(state.rows, 'no_date')}
      />
      {example ? (
        <OrderQuestion example={example} order={date.order} onChange={(order) => setAnswers('date', { date: { ...date, order } })} />
      ) : null}
      {unreadable > 0 ? (
        <SkippedNote>{pluralPick(t('transactions.import_wizard.date.skipped'), unreadable, i18n.language)}</SkippedNote>
      ) : null}
    </div>
  )
}
