import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { useIsPhone } from '@/hooks/useIsPhone'
import { calendarLocale } from '@/lib/calendarLocale'
import { parseDateTime } from '@/lib/datetime'
import { cn } from '@/lib/utils'

interface TransactionDatePickerProps {
  value: string
  onChange: (day: Date) => void
  /** names the chip when a form holds several dates */
  label?: string
  /** shown on the chip while `value` is empty (an optional date) */
  placeholder?: string
  /** `field`: the date reads as the value of a card field (`CardField`), like a borderless input */
  variant?: 'chip' | 'field'
  className?: string
  disabled?: boolean
}

const fieldTriggerClass =
  'h-auto w-full min-w-0 justify-start rounded-none border-0 bg-transparent p-0 text-base font-normal shadow-none hover:bg-transparent md:text-sm focus-visible:ring-0 dark:bg-transparent dark:hover:bg-transparent'

// a field shows the date as the screen around it does ("Nov 11, 2026"); the chip keeps the typed form
function fieldDate(day: Date, lang: string): string {
  return new Intl.DateTimeFormat(lang, { day: 'numeric', month: 'short', year: 'numeric' }).format(day)
}

export function TransactionDatePicker({ value, onChange, label, placeholder, variant = 'chip', className, disabled = false }: TransactionDatePickerProps) {
  const { t, i18n } = useTranslation()
  const isPhone = useIsPhone()
  const [open, setOpen] = useState(false)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const selectedDate = value ? parseDateTime(value) : new Date()
  const currentYear = new Date().getFullYear()
  const trigger = (
    <Button
      ref={triggerRef}
      type="button"
      variant="secondary"
      className={cn(
        variant === 'field'
          ? `${fieldTriggerClass} ${value ? '' : 'text-muted-foreground'}`
          : 'h-7 rounded-lg bg-econumo-card px-2 text-xs font-normal max-sm:h-11 max-sm:px-3',
        className,
      )}
      aria-label={label ?? 'date'}
      disabled={disabled}
      aria-haspopup="dialog"
      aria-expanded={open}
      onClick={isPhone ? () => setOpen(true) : undefined}
    >
      {value ? (variant === 'field' ? fieldDate(selectedDate, i18n.language) : value) : placeholder}
    </Button>
  )
  const calendar = (
    <Calendar
      mode="single"
      captionLayout="dropdown"
      defaultMonth={selectedDate}
      startMonth={new Date(Math.min(currentYear - 100, selectedDate.getFullYear()), 0)}
      endMonth={new Date(Math.max(currentYear + 100, selectedDate.getFullYear()), 11)}
      weekStartsOn={1}
      locale={calendarLocale(i18n.language)}
      selected={value ? selectedDate : undefined}
      className={isPhone ? 'mx-auto bg-transparent p-0 [--cell-size:min(2.75rem,calc((100vw-2rem)/7))] [&_button[data-day]]:text-base' : undefined}
      classNames={isPhone ? {
        dropdown_root: 'relative rounded-md bg-econumo-card focus-within:ring-2 focus-within:ring-ring',
        dropdown: 'absolute inset-0 size-full cursor-pointer opacity-0 text-base',
        caption_label: 'flex h-11 items-center gap-2 rounded-md px-2 text-base font-medium [&>svg]:size-4 [&>svg]:text-muted-foreground',
      } : undefined}
      onSelect={(day) => {
        if (day) {
          onChange(day)
          setOpen(false)
        }
      }}
    />
  )

  if (isPhone) {
    return (
      <>
        {trigger}
        <ResponsiveDialog
          open={open}
          onOpenChange={setOpen}
          title={label ?? t('transactions.modal.form.date.label')}
          onCloseAutoFocus={(event) => {
            event.preventDefault()
            triggerRef.current?.focus()
          }}
          footer={<Button type="button" variant="secondary" className="w-full" onClick={() => setOpen(false)}>{t('common.button.cancel.label')}</Button>}
        >
          {calendar}
        </ResponsiveDialog>
      </>
    )
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent className="w-auto p-0" align={variant === 'field' ? 'start' : 'end'}>{calendar}</PopoverContent>
    </Popover>
  )
}
