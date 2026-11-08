import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { CalculatorInput } from '@/components/CalculatorInput'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { OptionSelect } from '@/components/OptionSelect'
import { formatDate } from '@/lib/datetime'
import type { RecurringSchedule } from '@/api/dto/recurring'
import { TransactionDatePicker } from '@/features/transactions/TransactionDatePicker'
import { SCHEDULE_ORDER } from './useDebtForm'

export type FieldErrors = Record<string, string | undefined>

const amountInputClass = `${cardFieldControlClass} text-base`

export function AmountField({
  id,
  label,
  value,
  symbol,
  error,
  onChange,
}: {
  id: string
  label: string
  value: string
  symbol: string
  error?: string | null
  onChange: (value: string) => void
}) {
  return (
    <CardField label={label} htmlFor={id} error={error}>
      <div className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <CalculatorInput id={id} className={amountInputClass} value={value} onChange={onChange} />
        </div>
        <span className="pt-0.5 text-sm text-muted-foreground">{symbol}</span>
      </div>
    </CardField>
  )
}

/** The payment and how often it repeats, read as one line: "2,000.00 $ every month ▾". */
export function PaymentField({
  id,
  value,
  symbol,
  schedule,
  error,
  onChange,
  onScheduleChange,
}: {
  id: string
  value: string
  symbol: string
  schedule: RecurringSchedule
  error?: string | null
  onChange: (value: string) => void
  onScheduleChange: (value: RecurringSchedule) => void
}) {
  const { t } = useTranslation()
  const label = t('debts.form.payment.label')
  const repeats = t('debts.form.schedule.label')
  return (
    <div role="group" aria-label={`${label}, ${repeats}`}>
      <CardField label={label} htmlFor={id} error={error}>
        {/* on a phone the schedule takes its own line under the amount, so the
            keypad row keeps the card's width */}
        <div className="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-x-2 gap-y-1 sm:grid-cols-[minmax(0,1fr)_auto_auto]">
          <CalculatorInput id={id} className={amountInputClass} value={value} onChange={onChange} />
          <span className="pt-0.5 text-sm text-muted-foreground">{symbol}</span>
          <div className="col-span-2 sm:col-span-1 [&_button]:h-auto [&_button]:gap-1 [&_button]:border-0 [&_button]:bg-transparent [&_button]:p-0 [&_button]:pt-0.5 [&_button]:text-sm [&_button]:text-econumo-link [&_button]:shadow-none [&_button]:focus-visible:ring-0">
            <OptionSelect
              id={`${id}-schedule`}
              aria-label={repeats}
              title={repeats}
              value={schedule}
              onValueChange={(v) => onScheduleChange(v as RecurringSchedule)}
              options={SCHEDULE_ORDER.map((s) => ({ value: s, label: t(`debts.form.schedule.every.${s}`) }))}
            />
          </div>
        </div>
      </CardField>
    </div>
  )
}

export function StartDateField({ value, error, onChange }: { value: string; error?: string; onChange: (value: string) => void }) {
  const { t } = useTranslation()
  const label = t('debts.form.start_date.label')
  return (
    <CardField label={label} error={error}>
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs text-muted-foreground">{t('debts.form.start_date.hint')}</span>
        <span className="flex shrink-0 items-center gap-1">
          <TransactionDatePicker ariaLabel={label} placeholder={t('debts.form.start_date.none')} value={value} onChange={(day) => onChange(formatDate(day))} />
          {value ? (
            <Button type="button" variant="ghost" size="icon" className="size-7" aria-label={t('debts.form.start_date.clear')} onClick={() => onChange('')}>
              <X className="size-4" />
            </Button>
          ) : null}
        </span>
      </div>
    </CardField>
  )
}
