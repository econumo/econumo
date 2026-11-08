import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { Label } from '@/components/ui/label'
import { OptionSelect } from '@/components/OptionSelect'
import { formatDate } from '@/lib/datetime'
import type { RecurringSchedule } from '@/api/dto/recurring'
import { TransactionDatePicker } from '@/features/transactions/TransactionDatePicker'
import { SCHEDULE_ORDER } from './useDebtForm'

export type FieldErrors = Record<string, string | undefined>

const amountInputClass = `${cardFieldControlClass} text-base`

/** a plain amount: the decimal keyboard on phones, no calculator */
function AmountInput({ id, value, onChange }: { id: string; value: string; onChange: (value: string) => void }) {
  return <Input id={id} className={amountInputClass} inputMode="decimal" autoComplete="off" value={value} onChange={(e) => onChange(e.target.value)} />
}

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
      <div className="flex items-baseline gap-2">
        <div className="min-w-0 flex-1">
          <AmountInput id={id} value={value} onChange={onChange} />
        </div>
        <span className="text-sm text-muted-foreground">{symbol}</span>
      </div>
    </CardField>
  )
}

/** The payment and how often it repeats on one row, on every breakpoint: the
    amount card and, beside it, the same-height "every month ▾" card. */
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
    <div className="flex flex-col gap-1">
      <div role="group" aria-label={`${label}, ${repeats}`} className="flex items-stretch gap-2">
        <div className="flex min-w-0 flex-[3] flex-col gap-0.5 rounded-lg bg-econumo-card px-4 py-2.5 focus-within:ring-1 focus-within:ring-ring">
          <Label htmlFor={id} className="text-[11px] font-normal text-muted-foreground">
            {label}
          </Label>
          <div className="flex items-baseline gap-2">
            <div className="min-w-0 flex-1">
              <AmountInput id={id} value={value} onChange={onChange} />
            </div>
            <span className="text-sm text-muted-foreground">{symbol}</span>
          </div>
        </div>
        <div className="flex min-w-0 flex-[2] items-center rounded-lg bg-econumo-card px-3 focus-within:ring-1 focus-within:ring-ring">
          <OptionSelect
            id={`${id}-schedule`}
            className="h-auto w-full min-w-0 justify-between gap-1 border-0 bg-transparent p-0 text-sm text-econumo-link shadow-none focus-visible:ring-0 dark:bg-transparent dark:hover:bg-transparent"
            aria-label={repeats}
            title={repeats}
            value={schedule}
            onValueChange={(v) => onScheduleChange(v as RecurringSchedule)}
            options={SCHEDULE_ORDER.map((s) => ({ value: s, label: t(`debts.form.schedule.every.${s}`) }))}
          />
        </div>
      </div>
      {error ? <p className="px-1 text-sm text-destructive">{error}</p> : null}
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
