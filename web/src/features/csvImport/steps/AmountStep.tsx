import { useId, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Switch } from '@/components/ui/switch'
import { isNegative } from '@/lib/decimal'
import { pluralPick } from '@/lib/plural'
import { cn } from '@/lib/utils'
import { formatSigned, useAccountView } from '../accountView'
import { ColumnChoices } from '../ColumnChoices'
import { columnTags } from '../columnLabels'
import { Example, Field, Segmented, SkippedNote } from '../controls'
import { detectDecimalSeparator, parseAmountCell, type DecimalSeparator } from '../amounts'
import { columnValues } from '../guess'
import type { ParsedFile } from '../parse'
import type { WizardAnswers } from '../prepare'
import { missingAnswer, type StepBodyProps } from '../wizard'

type Amount = WizardAnswers['amount']

// A real value from the file: its first negative amount, else its first amount,
// written as the file writes it, with the sign the current setting gives it.
function spendingExample(values: string[], amount: Amount): string | null {
  let example: string | null = null
  for (const value of values) {
    const read = parseAmountCell(value, amount.sep)
    if (read === null) continue
    if (isNegative(read)) {
      example = value
      break
    }
    example ??= value
  }
  return example === null ? null : amount.flip ? flipWritten(example) : example
}

// the sign is turned around in the file's own writing, so "-12,50" reads "12,50", not "12.50";
// currency marks around the number ("EUR -12,50", "12,50 €") stay where they are
function flipWritten(value: string): string {
  if (/\(.*\d.*\)/.test(value)) return value.replace(/\((.*)\)/, '$1')
  if (/[-−]\s*(?=[\d.,])/.test(value)) return value.replace(/[-−]\s*(?=[\d.,])/, '')
  if (/\d\s*[-−]/.test(value)) return value.replace(/(\d)\s*[-−]/, '$1')
  return value.replace(/\+?(?=[\d.,])/, '-')
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

const EXAMPLE_ROWS = 4
const isNumber = (type: string) => type === 'number'

export function AmountStep({ state, setAnswers }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const flipId = useId()
  const title = t('transactions.import_wizard.amount.title')
  const { amount } = state.answers
  const set = (patch: Partial<Amount>) => setAnswers('amount', { amount: { ...amount, ...patch } })
  const accountView = useAccountView(state.answers)

  const values = useMemo(() => amountValues(state.file, amount), [state.file, amount])
  const separator = useMemo(() => detectDecimalSeparator(values), [values])
  const spending = amount.mode === 'single' && amount.column !== null ? spendingExample(values, amount) : null
  const separatorExample = separator.ambiguous ? separator.example : null
  const examples = state.rows.filter((row) => row.status === 'ok').slice(0, EXAMPLE_ROWS)

  const moneyIn = t('transactions.import_wizard.amount.money_in')
  const moneyOut = t('transactions.import_wizard.amount.money_out')
  const separatorQuestion = separatorExample ? t('transactions.import_wizard.amount.separator_question', { example: separatorExample }) : ''
  const missing = missingAnswer('amount', state.answers)
  const unreadable = state.rows.filter((row) => row.reason === 'no_amount').length

  return (
    <div className="flex flex-col gap-6">
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
          label={title}
          selected={amount.column}
          onSelect={(column) => set({ column })}
          fits={isNumber}
          tags={columnTags(state.answers, t, i18n.language, ['amount'])}
        />
      ) : (
        <div className="flex flex-col gap-4">
          <Field label={moneyIn}>
            <ColumnChoices
              profiles={state.profiles}
              label={moneyIn}
              selected={amount.inColumn}
              onSelect={(inColumn) => set({ inColumn })}
              fits={isNumber}
              tags={columnTags(state.answers, t, i18n.language, ['amountIn'])}
              noneLabel={t('transactions.import_wizard.amount.none_in')}
            />
          </Field>
          <Field label={moneyOut}>
            <ColumnChoices
              profiles={state.profiles}
              label={moneyOut}
              selected={amount.outColumn}
              onSelect={(outColumn) => set({ outColumn })}
              fits={isNumber}
              tags={columnTags(state.answers, t, i18n.language, ['amountOut'])}
              noneLabel={t('transactions.import_wizard.amount.none_out')}
            />
          </Field>
        </div>
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
      {examples.length === 0 ? (
        <Example>{t('transactions.import_wizard.amount.example_empty')}</Example>
      ) : (
        <ul aria-label={t('transactions.import_wizard.amount.example_label')} className="rounded-xl border border-border">
          {examples.map((row) => {
            const signed = formatSigned(row.amount ?? '0', accountView(row).currency)
            return (
              <li key={row.fileRow} className="flex items-center justify-between gap-3 border-b border-border px-3 py-2 text-sm last:border-0">
                <span className="truncate text-muted-foreground">{row.description || '—'}</span>
                <span className={cn('shrink-0 tabular-nums', signed.income && 'text-income')}>{signed.text}</span>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
