import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { useIsPhone } from '@/hooks/useIsPhone'
import { calendarLocale } from '@/lib/calendarLocale'
import { parseDateTime } from '@/lib/datetime'

export function TransactionDatePicker({ value, onChange }: { value: string; onChange: (day: Date) => void }) {
  const { t, i18n } = useTranslation()
  const isPhone = useIsPhone()
  const [open, setOpen] = useState(false)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const selectedDate = parseDateTime(value)
  const currentYear = new Date().getFullYear()
  const trigger = (
    <Button
      ref={triggerRef}
      type="button"
      variant="secondary"
      className="h-7 rounded bg-econumo-card px-2 text-xs font-normal max-sm:h-11 max-sm:px-3"
      aria-label="date"
      aria-haspopup="dialog"
      aria-expanded={open}
      onClick={isPhone ? () => setOpen(true) : undefined}
    >
      {value}
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
      selected={selectedDate}
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
          title={t('transactions.modal.form.date.label')}
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
      <PopoverContent className="w-auto p-0" align="end">{calendar}</PopoverContent>
    </Popover>
  )
}
