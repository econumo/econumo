import { useTranslation } from 'react-i18next'
import { CommandEmpty, CommandItem } from '@/components/ui/command'
import { PickerSheet } from '@/components/PickerSheet'
import { fullCurrencyLabel } from '@/components/CurrencySelect'
import { rankByName } from '@/lib/search'
import { useCurrencies } from '@/features/currencies/queries'
import { selectableCurrencies } from '@/features/currencies/selectable'
import type { Id } from '@/api/types'

interface CurrencyPickerDialogProps {
  open: boolean
  title: string
  value: Id | null
  onClose: () => void
  onPick: (id: Id) => void
}

// Search-first currency picker in a dialog (bottom sheet on mobile).
export function CurrencyPickerDialog({ open, title, value, onClose, onPick }: CurrencyPickerDialogProps) {
  const { t } = useTranslation()
  const { data: currencies } = useCurrencies()
  const options = (search: string) => rankByName(selectableCurrencies(currencies, value ?? undefined), (c) => [c.name, c.code, c.symbol], search)
  const pick = (id: Id) => {
    onPick(id)
    onClose()
  }

  return (
    <PickerSheet open={open} onClose={onClose} title={title} searchLabel={t('accounts.page.toolbar.search')} currentValue={value ?? undefined}>
      {(search) => (
        <>
          <CommandEmpty />
          {options(search).map((currency) => (
            <CommandItem key={currency.id} value={currency.id} data-checked={currency.id === value} onSelect={() => pick(currency.id)}>
              <span className="flex-1">{fullCurrencyLabel(currency)}</span>
            </CommandItem>
          ))}
        </>
      )}
    </PickerSheet>
  )
}
