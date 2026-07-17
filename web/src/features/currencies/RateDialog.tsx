import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { isNotEmpty } from '@/lib/validation'
import type { CurrencyDto } from '@/api/dto/currency'

interface RateDialogProps {
  open: boolean
  currency?: CurrencyDto | null
  serverError?: string | null
  onClose: () => void
  onSubmit: (form: { rate: string; date?: string }) => void
}

export function RateDialog({ open, currency, serverError, onClose, onSubmit }: RateDialogProps) {
  const { t } = useTranslation()
  const [rate, setRate] = useState('')
  const [date, setDate] = useState('')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (open) {
      setRate('')
      setDate('')
      setError(null)
    }
  }, [open, currency])

  const submit = () => {
    if (!isNotEmpty(rate)) {
      setError(t('classifications.currencies.forms.currency.name.validation.required_field'))
      return
    }
    onSubmit({ rate, date: date || undefined })
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={t('classifications.currencies.modals.rate.header')}
      footer={
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" form="rate-dialog-form">
            {t('classifications.currencies.modals.rate.submit')}
          </Button>
        </div>
      }
    >
      <form
        id="rate-dialog-form"
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div className="flex flex-col gap-2">
          <Label htmlFor="rate-value">{t('classifications.currencies.forms.currency.rate.label')}</Label>
          <Input id="rate-value" value={rate} onChange={(e) => setRate(e.target.value)} />
          {error ?? serverError ? <p className="text-sm text-destructive">{error ?? serverError}</p> : null}
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="rate-date">{t('classifications.currencies.forms.currency.date.label')}</Label>
          <Input id="rate-date" type="date" value={date} onChange={(e) => setDate(e.target.value)} />
        </div>
      </form>
    </ResponsiveDialog>
  )
}
