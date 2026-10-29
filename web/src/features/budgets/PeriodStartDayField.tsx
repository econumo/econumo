import { useTranslation } from 'react-i18next'
import { CardField } from '@/components/CardField'
import { OptionSelect } from '@/components/OptionSelect'

const DAYS = Array.from({ length: 28 }, (_, i) => ({ value: String(i + 1), label: String(i + 1) }))

interface PeriodStartDayFieldProps {
  id: string
  value: number
  onChange: (day: number) => void
  disabled?: boolean
  /** the change warning, shown while the value differs from the stored one */
  warning?: boolean
  error?: string | null
}

export function PeriodStartDayField({ id, value, onChange, disabled, warning, error }: PeriodStartDayFieldProps) {
  const { t } = useTranslation()
  const label = t('budgets.form.budget.period_start_day.label')
  return (
    <div className="flex flex-col gap-1">
      <CardField label={label} htmlFor={id} error={error}>
        <OptionSelect
          id={id}
          aria-label={label}
          title={label}
          className="w-full"
          value={String(value)}
          onValueChange={(v) => onChange(Number(v))}
          options={DAYS}
          disabled={disabled}
        />
      </CardField>
      {warning ? <p className="px-1 text-sm text-muted-foreground">{t('budgets.form.budget.period_start_day.warning')}</p> : null}
    </div>
  )
}
