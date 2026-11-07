import { useState } from 'react'
import { ChevronDownIcon } from 'lucide-react'
import { CommandItem } from '@/components/ui/command'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue, selectTriggerClass } from '@/components/ui/select'
import { PickerSheet } from '@/components/PickerSheet'
import { useIsPhone } from '@/hooks/useIsPhone'
import { cn } from '@/lib/utils'

interface OptionSelectProps {
  id?: string
  'aria-label': string
  /** the phone sheet's heading */
  title: string
  value: string
  onValueChange: (value: string) => void
  options: { value: string; label: string }[]
  className?: string
  disabled?: boolean
}

/** A short, fixed list (a status, a schedule, a month): the dropdown on a
    desktop or tablet; on a phone the same picker sheet as every other picker,
    without a search, so a choice looks and works alike everywhere */
export function OptionSelect({ id, 'aria-label': ariaLabel, title, value, onValueChange, options, className, disabled }: OptionSelectProps) {
  const isPhone = useIsPhone()
  const [open, setOpen] = useState(false)

  if (!isPhone) {
    return (
      <Select value={value} onValueChange={onValueChange} disabled={disabled}>
        <SelectTrigger id={id} aria-label={ariaLabel} className={className}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    )
  }

  return (
    <>
      <button
        type="button"
        id={id}
        aria-label={ariaLabel}
        aria-haspopup="dialog"
        aria-expanded={open}
        disabled={disabled}
        data-slot="select-trigger"
        data-size="default"
        className={cn(selectTriggerClass, className)}
        onClick={() => setOpen(true)}
      >
        <span data-slot="select-value">{options.find((o) => o.value === value)?.label ?? ''}</span>
        <ChevronDownIcon className="pointer-events-none size-4 text-muted-foreground" />
      </button>
      <PickerSheet open={open} onClose={() => setOpen(false)} title={title} searchLabel={title} searchable={false} currentValue={value}>
        {() =>
          options.map((option) => (
            <CommandItem
              key={option.value}
              value={option.value}
              data-checked={option.value === value}
              onSelect={() => {
                onValueChange(option.value)
                setOpen(false)
              }}
            >
              {option.label}
            </CommandItem>
          ))
        }
      </PickerSheet>
    </>
  )
}
