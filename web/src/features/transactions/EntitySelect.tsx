import { useRef, useState } from 'react'
import { ChevronsUpDown, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Command, CommandEmpty, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { EntityIcon } from '@/components/EntityIcon'
import { useIsMobile } from '@/hooks/useIsMobile'

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
  const isMobile = useIsMobile()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const triggerRef = useRef<HTMLButtonElement | null>(null)
  // inside the vaul drawer the popover must portal INTO the sheet: vaul's touch
  // lock swallows touchmove on anything portaled to body, so the list would not
  // scroll on mobile otherwise
  const portalContainer = isMobile
    ? (triggerRef.current?.closest('[data-slot="drawer-content"], [data-slot="dialog-content"]') as HTMLElement | null) ?? undefined
    : undefined

  const selected = options.find((o) => o.value === value)
  const filtered = options.filter((o) => !search || o.label.toLowerCase().includes(search.toLowerCase()))
  const exactMatch = options.some((o) => o.label.toLowerCase() === search.toLowerCase())
  const canCreate = !!onCreate && search !== '' && !exactMatch && (createValidator ? createValidator(search) : true)

  return (
    // modal on desktop only: the dialog's scroll lock otherwise swallows wheel
    // events over the portaled popover, making the list unscrollable; inside
    // the vaul drawer a modal popover dismisses itself immediately
    <Popover modal={!isMobile} open={open} onOpenChange={(next) => { setOpen(next); setSearch('') }}>
      <PopoverTrigger asChild>
        <Button
          ref={triggerRef}
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
      <PopoverContent
        // default open-autofocus lands on the search input everywhere — on
        // mobile that pops the keyboard immediately, which is the point:
        // pick-by-typing without a second tap
        className="flex max-h-[min(20rem,var(--radix-popover-content-available-height))] w-[max(var(--radix-popover-trigger-width),14rem)] flex-col p-0 max-md:max-h-[min(26rem,var(--radix-popover-content-available-height))]"
        align="start"
        container={portalContainer}
      >
        <Command shouldFilter={false} className="min-h-0">
          <CommandInput
            value={search}
            onValueChange={setSearch}
            placeholder={onCreate ? t('elements.select.create_placeholder') : t('elements.select.search_placeholder')}
          />
          <CommandList className="mt-1 max-h-none min-h-0 flex-1 overflow-y-auto">
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
                className="text-econumo-magenta data-[selected=true]:text-econumo-magenta"
                onSelect={() => {
                  onCreate?.(search)
                  setOpen(false)
                }}
              >
                <Plus className="size-4 text-econumo-magenta" />
                {t('elements.button.add.label')} «{search}»
              </CommandItem>
            ) : null}
          </CommandList>
          {/* creation happens by typing a new name — say so instead of hiding it */}
          {onCreate && !canCreate ? (
            <div className="mt-1 flex items-center gap-1.5 border-t px-3 py-2 text-xs text-muted-foreground">
              <Plus className="size-3.5" />
              {t('elements.select.create_hint')}
            </div>
          ) : null}
        </Command>
      </PopoverContent>
    </Popover>
  )
}
