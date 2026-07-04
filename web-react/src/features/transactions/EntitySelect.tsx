import { useState } from 'react'
import { ChevronsUpDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Command, CommandEmpty, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { EntityIcon } from '@/components/EntityIcon'

export interface EntityOption {
  value: string
  label: string
  icon?: string
}

interface EntitySelectProps {
  value: string | null
  onChange: (value: string | null) => void
  options: EntityOption[]
  id?: string
  'aria-label'?: string
  placeholder?: string
  clearable?: boolean
  disabled?: boolean
  /** offered when the typed text matches no option and passes validation */
  onCreate?: (name: string) => void
  createValidator?: (name: string) => boolean
}

// Filter-as-you-type entity picker (category/payee) with optional create-on-the-fly,
// mirroring the Vue q-select use-input + @new-value behavior.
export function EntitySelect({
  value,
  onChange,
  options,
  id,
  'aria-label': ariaLabel,
  placeholder,
  clearable,
  disabled,
  onCreate,
  createValidator,
}: EntitySelectProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')

  const selected = options.find((o) => o.value === value)
  const filtered = options.filter((o) => !search || o.label.toLowerCase().includes(search.toLowerCase()))
  const exactMatch = options.some((o) => o.label.toLowerCase() === search.toLowerCase())
  const canCreate = !!onCreate && search !== '' && !exactMatch && (createValidator ? createValidator(search) : true)

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
          <span className="flex min-w-0 items-center gap-2">
            {selected?.icon ? <EntityIcon name={selected.icon} className="text-base text-muted-foreground" /> : null}
            <span className="truncate">{selected?.label ?? placeholder ?? ''}</span>
          </span>
          <ChevronsUpDown className="size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="p-0" align="start">
        <Command shouldFilter={false}>
          <CommandInput value={search} onValueChange={setSearch} />
          <CommandList>
            <CommandEmpty />
            {clearable && value ? (
              <CommandItem
                value="__clear__"
                onSelect={() => {
                  onChange(null)
                  setOpen(false)
                }}
              >
                —
              </CommandItem>
            ) : null}
            {filtered.map((option) => (
              <CommandItem
                key={option.value}
                value={option.value}
                onSelect={() => {
                  onChange(option.value)
                  setOpen(false)
                }}
              >
                {option.icon ? <EntityIcon name={option.icon} className="text-base text-muted-foreground" /> : null}
                {option.label}
              </CommandItem>
            ))}
            {canCreate ? (
              <CommandItem
                value={`__create__${search}`}
                onSelect={() => {
                  onCreate?.(search)
                  setOpen(false)
                }}
              >
                {t('elements.button.add.label')} «{search}»
              </CommandItem>
            ) : null}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
