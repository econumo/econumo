import { useId, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Switch } from '@/components/ui/switch'
import { pluralPick } from '@/lib/plural'
import { cn } from '@/lib/utils'
import { formatSigned, useAccountView } from '../accountView'
import { ColumnChoices } from '../ColumnChoices'
import { columnTags, fromName } from '../columnLabels'
import { candidates } from '../candidates'
import { AnswerLines, AnswerRow, ChoicesDone, Field, Segmented, SkippedNote } from '../controls'
import { detectDecimalSeparator, parseAmountCell, type DecimalSeparator } from '../amounts'
import { columnValues } from '../guess'
import type { ParsedFile } from '../parse'
import type { WizardAnswers } from '../prepare'
import { hasGuess, missingAnswer, type StepBodyProps } from '../wizard'

type Amount = WizardAnswers['amount']

// a fixed round example in the file's decimal notation: a real value from the file (often "-1") reads oddly
function spendingExample(amount: Amount): string {
  const value = `100${amount.sep}00`
  return amount.flip ? value : `−${value}`
}

function readAs(value: string, sep: DecimalSeparator, language: string): string {
  const parsed = parseAmountCell(value, sep)
  if (parsed === null) return value
  return new Intl.NumberFormat(language, { useGrouping: false, maximumFractionDigits: 20 }).format(Number(parsed))
}

function amountValues(file: ParsedFile, amount: Amount): string[] {
  const columns = amount.mode === 'single' ? [amount.column] : [amount.inColumn, amount.outColumn]
  return columns.flatMap((column) => columnValues(file, column))
}

const EXAMPLE_ROWS = 3

export function AmountStep({ state, setAnswers, choosing, change, closeChoices, keepChoices }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const flipId = useId()
  const { amount } = state.answers
  const set = (patch: Partial<Amount>) => setAnswers('amount', { amount: { ...amount, ...patch } })
  const accountView = useAccountView(state.answers)
  // money in and money out are two picks: the list folds once both sides were picked, not after the first
  const picked = useRef(new Set<'in' | 'out'>())
  const pickSide = (side: 'in' | 'out', patch: Partial<Amount>) => {
    set(patch)
    picked.current.add(side)
    if (picked.current.size === 2) {
      picked.current = new Set()
      closeChoices()
    } else {
      keepChoices()
    }
  }

  const values = useMemo(() => amountValues(state.file, amount), [state.file, amount])
  const separator = useMemo(() => detectDecimalSeparator(values), [values])
  const spending = amount.mode === 'single' && amount.column !== null ? spendingExample(amount) : null
  const separatorExample = separator.ambiguous ? separator.example : null
  // every row whose amount reads, also one left out for its date: hasGuess counts the same rows
  const readable = state.rows.filter((row) => row.amount !== null)
  const examples = readable.slice(0, EXAMPLE_ROWS)
  const guessed = hasGuess('amount', state)
  const open = choosing || !guessed
  if (!open) picked.current.clear()
  const nameOf = (column: number | null) => (column === null ? null : fromName(state.profiles[column], state.profiles, t))
  const from =
    amount.mode === 'single'
      ? nameOf(amount.column)
      : [nameOf(amount.inColumn), nameOf(amount.outColumn)].filter((name) => name !== null).join(' · ') || null

  const moneyIn = t('transactions.import_wizard.amount.choose_in')
  const moneyOut = t('transactions.import_wizard.amount.choose_out')
  const separatorQuestion = separatorExample ? t('transactions.import_wizard.amount.separator_question', { example: separatorExample }) : ''
  const missing = missingAnswer('amount', state.answers)
  const unreadable = state.rows.filter((row) => row.reason === 'no_amount').length

  return (
    <div className="flex flex-col gap-6">
      {open ? (
        <div className="flex flex-col gap-4">
          <Segmented<Amount['mode']>
            label={t('transactions.import_wizard.amount.mode')}
            value={amount.mode}
            options={[
              { value: 'single', label: t('transactions.import_wizard.amount.single') },
              { value: 'dual', label: t('transactions.import_wizard.amount.dual') },
            ]}
            onChange={(mode) => set({ mode })}
          />
          {amount.mode === 'single' ? (
            <ColumnChoices
              profiles={state.profiles}
              label={t('transactions.import_wizard.amount.choose')}
              selected={amount.column}
              onSelect={(column) => {
                set({ column })
                closeChoices()
              }}
              candidates={candidates('amount', state.profiles, state.answers)}
              nothingFitsNote={t('transactions.import_wizard.nothing_fits.amount')}
              onlyOneNote={t('transactions.import_wizard.only_one.amount')}
              tags={columnTags(state.answers, t, i18n.language, ['amount'])}
            />
          ) : (
            <>
              <ColumnChoices
                profiles={state.profiles}
                label={moneyIn}
                selected={amount.inColumn}
                onSelect={(inColumn) => pickSide('in', { inColumn })}
                candidates={candidates('amountIn', state.profiles, state.answers)}
                nothingFitsNote={t('transactions.import_wizard.nothing_fits.amount')}
                onlyOneNote={t('transactions.import_wizard.only_one.amount')}
                tags={columnTags(state.answers, t, i18n.language, ['amountIn'])}
                noneLabel={t('transactions.import_wizard.amount.none_in')}
                unanswered={!state.touched.has('amount') && amount.inColumn === null}
              />
              <ColumnChoices
                profiles={state.profiles}
                label={moneyOut}
                selected={amount.outColumn}
                onSelect={(outColumn) => pickSide('out', { outColumn })}
                candidates={candidates('amountOut', state.profiles, state.answers)}
                nothingFitsNote={t('transactions.import_wizard.nothing_fits.amount')}
                onlyOneNote={t('transactions.import_wizard.only_one.amount')}
                tags={columnTags(state.answers, t, i18n.language, ['amountOut'])}
                noneLabel={t('transactions.import_wizard.amount.none_out')}
                unanswered={!state.touched.has('amount') && amount.outColumn === null}
              />
              {guessed ? <ChoicesDone onDone={closeChoices} /> : null}
            </>
          )}
        </div>
      ) : (
        <AnswerRow from={from} onChange={change}>
          <AnswerLines
            label={t('transactions.import_wizard.amount.example_label')}
            lines={examples.map((row) => {
              const signed = formatSigned(row.amount ?? '0', accountView(row).currency)
              return <span className={cn('tabular-nums', signed.income && 'text-income')}>{signed.text}</span>
            })}
            more={readable.length - examples.length}
          />
        </AnswerRow>
      )}
      {unreadable > 0 && !missing ? (
        <SkippedNote>{pluralPick(t('transactions.import_wizard.amount.skipped'), unreadable, i18n.language)}</SkippedNote>
      ) : null}
      {separatorExample || spending !== null ? (
        <div className="grid gap-5 sm:grid-cols-2">
          {separatorExample ? (
            <Field label={separatorQuestion}>
              <Segmented<DecimalSeparator>
                label={separatorQuestion}
                value={amount.sep}
                options={[
                  { value: '.', label: readAs(separatorExample, '.', i18n.language) },
                  { value: ',', label: readAs(separatorExample, ',', i18n.language) },
                ]}
                onChange={(sep) => set({ sep })}
              />
            </Field>
          ) : null}
          {spending !== null ? (
            <Field label={t('transactions.import_wizard.amount.spending', { example: spending })} hint={t('transactions.import_wizard.amount.flip_hint')}>
              <label htmlFor={flipId} className="flex items-center gap-2 text-sm">
                <Switch id={flipId} checked={amount.flip} onCheckedChange={(flip) => set({ flip })} />
                {t('transactions.import_wizard.amount.flip')}
              </label>
            </Field>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
