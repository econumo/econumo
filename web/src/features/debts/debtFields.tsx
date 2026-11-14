import { useState } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { EntityIcon } from '@/components/EntityIcon'
import { IconPicker } from '@/components/IconPicker'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { Label } from '@/components/ui/label'
import { OptionSelect } from '@/components/OptionSelect'
import { formatDate } from '@/lib/datetime'
import type { RecurringSchedule } from '@/api/dto/recurring'
import { TransactionDatePicker } from '@/features/transactions/TransactionDatePicker'
import { SCHEDULE_ORDER } from './useDebtForm'

export type FieldErrors = Record<string, string | undefined>

const amountInputClass = `${cardFieldControlClass} text-base`

/** the debt's name, its icon a button at the start of the input that opens
    the icon list; picking one closes it */
export function DebtNameField({
  id,
  name,
  icon,
  error,
  onNameChange,
  onIconChange,
}: {
  id: string
  name: string
  icon: string
  error?: string
  onNameChange: (name: string) => void
  onIconChange: (icon: string) => void
}) {
  const { t } = useTranslation()
  const [picking, setPicking] = useState(false)
  return (
    <>
      <CardField label={t('debts.form.name.label')} htmlFor={id} error={error}>
        <div className="flex items-center gap-2">
          <button
            type="button"
            className="grid size-7 shrink-0 place-items-center rounded-md text-muted-foreground hover:bg-primary/10 hover:text-primary"
            aria-label={t('debts.form.icon.label')}
            onClick={() => setPicking(true)}
          >
            <EntityIcon name={icon} className="text-lg" />
          </button>
          <Input id={id} className={cardFieldControlClass} autoComplete="off" value={name} onChange={(e) => onNameChange(e.target.value)} />
        </div>
      </CardField>
      <ResponsiveDialog open={picking} onOpenChange={setPicking} title={t('debts.form.icon.label')}>
        <IconPicker
          value={icon}
          onChange={(next) => {
            onIconChange(next)
            setPicking(false)
          }}
          aria-label={t('debts.form.icon.label')}
        />
      </ResponsiveDialog>
    </>
  )
}

/** a plain amount: the decimal keyboard on phones, no calculator */
function AmountInput({ id, value, onChange }: { id: string; value: string; onChange: (value: string) => void }) {
  return <Input id={id} className={amountInputClass} inputMode="decimal" autoComplete="off" value={value} onChange={(e) => onChange(e.target.value)} />
}

export function AmountField({
  id,
  label,
  hint,
  value,
  symbol,
  error,
  echo,
  onChange,
}: {
  id: string
  label: string
  /** a line under the card saying when to fill it */
  hint?: string
  value: string
  symbol: string
  error?: string | null
  /** how the typed amount is read (`amountEcho`), shown while there is no error */
  echo?: string | null
  onChange: (value: string) => void
}) {
  return (
    <div className="flex flex-col gap-1">
      <CardField label={label} htmlFor={id} error={error}>
        <div className="flex items-baseline gap-2">
          <div className="min-w-0 flex-1">
            <AmountInput id={id} value={value} onChange={onChange} />
          </div>
          <span className="text-sm text-muted-foreground">{symbol}</span>
        </div>
      </CardField>
      {!error ? <AmountEcho echo={echo} /> : null}
      {hint ? <p className="px-1 text-xs text-muted-foreground">{hint}</p> : null}
    </div>
  )
}

function AmountEcho({ echo }: { echo?: string | null }) {
  return echo ? <p className="px-1 text-xs text-muted-foreground">{echo}</p> : null
}

/** The payment and how often it repeats on one row, on every breakpoint: the
    amount card and, beside it, the same-height "every month ▾" card. */
export function PaymentField({
  id,
  value,
  symbol,
  schedule,
  error,
  echo,
  onChange,
  onScheduleChange,
}: {
  id: string
  value: string
  symbol: string
  schedule: RecurringSchedule
  error?: string | null
  /** how the typed amount is read (`amountEcho`), shown while there is no error */
  echo?: string | null
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
      {error ? <p className="px-1 text-sm text-destructive">{error}</p> : <AmountEcho echo={echo} />}
    </div>
  )
}

export function StartDateField({ value, error, onChange }: { value: string; error?: string; onChange: (value: string) => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col gap-1">
      <DateField
        label={t('debts.form.start_date.label')}
        value={value}
        placeholder={t('debts.form.start_date.none')}
        error={error}
        onChange={onChange}
        onClear={() => onChange('')}
        clearLabel={t('debts.form.start_date.clear')}
      />
      <p className="px-1 text-xs text-muted-foreground">{t('debts.form.start_date.hint')}</p>
    </div>
  )
}

/** One date as a card field, laid out like the amount fields: the label above,
    the date under it in the same type as a typed value, the hint for screen
    readers only (a 360 px phone has no room for it). The date's wrapper is
    marked `data-date-chip`, for Finish setup's focus. */
export function DateField({
  label,
  hint,
  value,
  placeholder,
  error,
  onChange,
  onClear,
  clearLabel,
}: {
  label: string
  hint?: string
  value: string
  placeholder?: string
  error?: string | null
  onChange: (value: string) => void
  onClear?: () => void
  clearLabel?: string
}) {
  return (
    <div className="flex min-w-0 flex-1 flex-col gap-1">
      <div className="flex min-w-0 flex-1 flex-col gap-0.5 rounded-lg bg-econumo-card px-4 py-2.5 focus-within:ring-1 focus-within:ring-ring">
        <span className="truncate text-[11px] text-muted-foreground" title={hint}>
          {label}
        </span>
        {hint ? <span className="sr-only">{hint}</span> : null}
        <span data-date-chip className="flex min-w-0 items-center gap-1">
          <TransactionDatePicker variant="field" label={label} placeholder={placeholder} value={value} onChange={(day) => onChange(formatDate(day))} />
          {onClear && value ? (
            <Button type="button" variant="ghost" size="icon" className="-my-1 size-7 shrink-0" aria-label={clearLabel} onClick={onClear}>
              <X className="size-4" />
            </Button>
          ) : null}
        </span>
      </div>
      {error ? <p className="px-1 text-sm text-destructive">{error}</p> : null}
    </div>
  )
}

/** The start date and the last payment date on one row, on every breakpoint:
    the start date first, as the loan's dates read. `clearable` offers to drop
    the start date (a debt not set up, where it only bounds the payments). */
export function DatesRow({
  startDate,
  lastPaymentDate,
  startError,
  lastPaymentError,
  clearable = false,
  onStartDateChange,
  onLastPaymentDateChange,
}: {
  startDate: string
  lastPaymentDate: string
  startError?: string
  lastPaymentError?: string
  clearable?: boolean
  onStartDateChange: (value: string) => void
  onLastPaymentDateChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const startLabel = t('debts.form.start_date.label')
  const lastLabel = t('debts.form.last_payment_date.label')
  const error = startError ?? lastPaymentError
  return (
    <div className="flex flex-col gap-1">
      <div role="group" aria-label={`${startLabel}, ${lastLabel}`} className="flex items-stretch gap-2">
        <DateField
          label={startLabel}
          hint={t('debts.form.start_date.hint')}
          value={startDate}
          placeholder={t('debts.form.last_payment_date.choose')}
          onChange={onStartDateChange}
          onClear={clearable ? () => onStartDateChange('') : undefined}
          clearLabel={t('debts.form.start_date.clear')}
        />
        <DateField
          label={lastLabel}
          hint={t('debts.form.last_payment_date.hint')}
          value={lastPaymentDate}
          placeholder={t('debts.form.last_payment_date.choose')}
          onChange={onLastPaymentDateChange}
        />
      </div>
      {error ? <p className="px-1 text-sm text-destructive">{error}</p> : null}
    </div>
  )
}
