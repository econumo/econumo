import { useEffect, useState } from 'react'
import { Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Command, CommandEmpty, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { fullCurrencyLabel, fuzzyMatch } from '@/components/CurrencySelect'
import { useCurrencies } from '@/features/currencies/queries'
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
  const [search, setSearch] = useState('')

  useEffect(() => {
    if (open) {
      setSearch('')
    }
  }, [open])

  const options = (currencies ?? []).filter(
    (c) => !search || fuzzyMatch(c.name, search) || fuzzyMatch(c.symbol, search) || fuzzyMatch(c.code, search),
  )

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={title}>
      <Command shouldFilter={false}>
        <CommandInput autoFocus placeholder={t('pages.account.toolbar.search')} value={search} onValueChange={setSearch} />
        <CommandList className="mt-4 max-h-72 max-md:max-h-96">
          <CommandEmpty />
          {options.map((currency) => (
            <CommandItem
              key={currency.id}
              value={currency.id}
              onSelect={() => {
                onPick(currency.id)
                onClose()
              }}
            >
              <span className="flex-1">{fullCurrencyLabel(currency)}</span>
              {currency.id === value ? <Check className="size-4 text-econumo-magenta" /> : null}
            </CommandItem>
          ))}
        </CommandList>
      </Command>
    </ResponsiveDialog>
  )
}
