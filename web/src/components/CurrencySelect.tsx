import { useState } from 'react'
import { ChevronsUpDown } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Command, CommandEmpty, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { useCurrencies } from '@/features/currencies/queries'
import { selectableCurrencies } from '@/features/currencies/selectable'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'

// Port of the Vue CurrencySelect: subsequence ("fuzzy") match on name/symbol/code.
export function fuzzyMatch(str: string, pattern: string): boolean {
  const p = pattern.toLowerCase()
  const s = str.toLowerCase()
  let patternIdx = 0
  let strIdx = 0
  while (patternIdx < p.length && strIdx < s.length) {
    if (p[patternIdx] === s[strIdx]) {
      patternIdx++
    }
    strIdx++
  }
  return patternIdx === p.length
}

// "USD, $, US Dollar" with duplicates removed, exactly like the Vue option label.
export function fullCurrencyLabel(currency: CurrencyDto): string {
  return Array.from(new Set([currency.code, currency.symbol, currency.name])).join(', ')
}

interface CurrencySelectProps {
  value: Id | null
  onChange: (id: Id) => void
  disabled?: boolean
  id?: string
  'aria-label'?: string
}

export function CurrencySelect({ value, onChange, disabled, id, 'aria-label': ariaLabel }: CurrencySelectProps) {
  const { data: currencies } = useCurrencies()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')

  const selected = currencies?.find((c) => c.id === value)
  const options = selectableCurrencies(currencies, value ?? undefined).filter(
    (c) => !search || fuzzyMatch(c.name, search) || fuzzyMatch(c.symbol, search) || fuzzyMatch(c.code, search),
  )

  return (
    <Popover open={open} onOpenChange={(next) => { setOpen(next); setSearch('') }}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-label={ariaLabel}
          disabled={disabled}
          className="w-full justify-between font-normal"
        >
          {selected?.code ?? ''}
          <ChevronsUpDown className="size-4 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-(--radix-popover-trigger-width) p-0" align="start">
        <Command shouldFilter={false}>
          <CommandInput value={search} onValueChange={setSearch} />
          <CommandList>
            <CommandEmpty />
            {options.map((currency) => (
              <CommandItem
                key={currency.id}
                value={currency.id}
                onSelect={() => {
                  onChange(currency.id)
                  setOpen(false)
                }}
              >
                {fullCurrencyLabel(currency)}
              </CommandItem>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
