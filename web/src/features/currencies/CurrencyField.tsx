import { fullCurrencyLabel } from '@/components/CurrencySelect'
import { EntitySelect } from '@/features/transactions/EntitySelect'
import { SelectCard } from '@/features/transactions/SelectCard'
import type { Id } from '@/api/types'
import { useCurrencies } from './queries'
import { selectableCurrencies } from './selectable'

interface CurrencyFieldProps {
  label: string
  value: Id | null
  onChange: (id: Id) => void
  error?: string | null
  disabled?: boolean
}

/** A form's currency, picked like a transaction's category: an inline
    searchable dropdown on a desktop or tablet, the picker sheet on a phone */
export function CurrencyField({ label, value, onChange, error, disabled }: CurrencyFieldProps) {
  const { data: currencies } = useCurrencies()
  const options = selectableCurrencies(currencies, value ?? undefined).map((c) => ({ value: c.id, label: fullCurrencyLabel(c) }))
  return (
    <SelectCard label={label} error={error}>
      <EntitySelect
        aria-label={label}
        value={value}
        onChange={(id) => {
          // a currency is required: there is no clear row, so null never comes
          if (id) {
            onChange(id)
          }
        }}
        options={options}
        disabled={disabled}
      />
    </SelectCard>
  )
}
