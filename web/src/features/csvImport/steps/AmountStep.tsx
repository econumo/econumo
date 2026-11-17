import { useId, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Switch } from '@/components/ui/switch'
import { isNegative } from '@/lib/decimal'
import { ChoiceCards, ColumnCards } from '../ColumnCards'
import { detectDecimalSeparator, parseAmountCell, signedAmount, type DecimalSeparator } from '../amounts'
import { columnValues } from '../guess'
import type { ParsedFile } from '../parse'
import type { WizardAnswers } from '../prepare'
import type { StepBodyProps } from '../wizard'
import { StepTitle } from './StepTitle'

type Amount = WizardAnswers['amount']

const SIGNS = /^[\s+\-−(]+|[\s\-−)]+$/g

// A value from the file the current settings read as spending. A file with no
// such value still gets an example: its first amount, written the way spending
// would have to look for the current setting.
function spendingExample(values: string[], amount: Amount): string | null {
  let first: string | null = null
  for (const value of values) {
    const read = signedAmount({ single: value }, 'single', amount.flip, amount.sep)
    if (read === null || !/[1-9]/.test(read)) continue
    if (isNegative(read)) return value
    first ??= value
  }
  if (first === null) return null
  const magnitude = first.replace(SIGNS, '')
  return amount.flip ? magnitude : `−${magnitude}`
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

export function AmountStep({ state, setAnswers }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const flipId = useId()
  const title = t('transactions.import_wizard.amount.title')
  const { amount } = state.answers
  const set = (patch: Partial<Amount>) => setAnswers('amount', { amount: { ...amount, ...patch } })

  const values = useMemo(() => amountValues(state.file, amount), [state.file, amount])
  const separator = useMemo(() => detectDecimalSeparator(values), [values])
  const spending = amount.mode === 'single' && amount.column !== null ? spendingExample(values, amount) : null
  const separatorExample = separator.ambiguous ? separator.example : null

  const moneyIn = t('transactions.import_wizard.amount.money_in')
  const moneyOut = t('transactions.import_wizard.amount.money_out')
  const separatorQuestion = separatorExample ? t('transactions.import_wizard.amount.separator_question', { example: separatorExample }) : ''

  return (
    <div className="flex flex-col gap-3">
      <StepTitle>{title}</StepTitle>
      <ChoiceCards<Amount['mode']>
        label={title}
        value={amount.mode}
        options={[
          { value: 'single', label: t('transactions.import_wizard.amount.single') },
          { value: 'dual', label: t('transactions.import_wizard.amount.dual') },
        ]}
        onChange={(mode) => set({ mode })}
      />
      {amount.mode === 'single' ? (
        <ColumnCards
          label={t('transactions.import_wizard.amount.column')}
          profiles={state.profiles}
          value={amount.column}
          onChange={(column) => set({ column })}
        />
      ) : (
        <>
          <p className="text-sm">{moneyIn}</p>
          <ColumnCards label={moneyIn} profiles={state.profiles} value={amount.inColumn} onChange={(inColumn) => set({ inColumn })} allowNone />
          <p className="text-sm">{moneyOut}</p>
          <ColumnCards label={moneyOut} profiles={state.profiles} value={amount.outColumn} onChange={(outColumn) => set({ outColumn })} allowNone />
        </>
      )}
      {separatorExample ? (
        <div className="flex flex-col gap-2">
          <p className="text-sm">{separatorQuestion}</p>
          <ChoiceCards<DecimalSeparator>
            label={separatorQuestion}
            value={amount.sep}
            options={[
              { value: '.', label: readAs(separatorExample, '.', i18n.language) },
              { value: ',', label: readAs(separatorExample, ',', i18n.language) },
            ]}
            onChange={(sep) => set({ sep })}
          />
        </div>
      ) : null}
      {spending !== null ? (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-3">
          <span className="text-sm">{t('transactions.import_wizard.amount.spending', { example: spending })}</span>
          <label htmlFor={flipId} className="flex items-center gap-2 text-sm">
            {t('transactions.import_wizard.amount.flip')}
            <Switch id={flipId} checked={amount.flip} onCheckedChange={(flip) => set({ flip })} />
          </label>
        </div>
      ) : null}
    </div>
  )
}
