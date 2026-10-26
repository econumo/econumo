import { useEffect, useRef, useState } from 'react'
import { Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Command, CommandEmpty, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { fullCurrencyLabel } from '@/components/CurrencySelect'
import { rankByName } from '@/lib/search'
import { SheetSearch } from '@/components/SheetSearch'
import { useIsPhone } from '@/hooks/useIsPhone'
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
  const [search, setSearch] = useState('')

  useEffect(() => {
    if (open) {
      setSearch('')
    }
  }, [open])

  const options = rankByName(selectableCurrencies(currencies, value ?? undefined), (c) => [c.name, c.code, c.symbol], search)
  const isPhone = useIsPhone()
  const searchRef = useRef<HTMLInputElement>(null)
  const pick = (id: Id) => {
    onPick(id)
    onClose()
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={title}
      initialFocus={searchRef}
      footer={
        isPhone ? (
          <SheetSearch
            ref={searchRef}
            aria-label={t('accounts.page.toolbar.search')}
            placeholder={t('accounts.page.toolbar.search')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && options[0]) {
                e.preventDefault()
                pick(options[0].id)
              }
            }}
          />
        ) : undefined
      }
    >
      <Command shouldFilter={false}>
        {isPhone ? null : <CommandInput autoFocus placeholder={t('accounts.page.toolbar.search')} value={search} onValueChange={setSearch} />}
        <CommandList className="mt-4 max-h-72 max-md:mt-0 max-md:max-h-none">
          <CommandEmpty />
          {options.map((currency) => (
            <CommandItem
              key={currency.id}
              value={currency.id}
              onSelect={() => pick(currency.id)}
            >
              <span className="flex-1">{fullCurrencyLabel(currency)}</span>
              {currency.id === value ? <Check className="size-4 text-econumo-link" /> : null}
            </CommandItem>
          ))}
        </CommandList>
      </Command>
    </ResponsiveDialog>
  )
}
