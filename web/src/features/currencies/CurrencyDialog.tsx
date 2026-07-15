import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { isNotEmpty } from '@/lib/validation'
import type { CurrencyDto } from '@/api/dto/currency'

export interface CurrencyDialogForm {
  code: string
  name: string
  symbol: string
  fractionDigits: number
  rate: string
}

interface CurrencyDialogProps {
  open: boolean
  currency?: CurrencyDto | null
  onClose: () => void
  onSubmit: (form: CurrencyDialogForm) => void
}

export function CurrencyDialog({ open, currency, onClose, onSubmit }: CurrencyDialogProps) {
  const { t } = useTranslation()
  const isNew = !currency
  const [code, setCode] = useState('')
  const [name, setName] = useState('')
  const [symbol, setSymbol] = useState('')
  const [fractionDigits, setFractionDigits] = useState(2)
  const [rate, setRate] = useState('')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (open) {
      setCode(currency?.code ?? '')
      setName(currency?.name ?? '')
      setSymbol(currency?.symbol ?? '')
      setFractionDigits(currency?.fractionDigits ?? 2)
      setRate('')
      setError(null)
    }
  }, [open, currency])

  const submit = () => {
    if (!isNotEmpty(name)) {
      setError(t('modules.classifications.currencies.forms.currency.name.validation.required_field'))
      return
    }
    onSubmit({ code, name, symbol, fractionDigits, rate })
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={isNew ? t('modules.classifications.currencies.modals.create.header') : t('modules.classifications.currencies.modals.edit.header')}
      footer={
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('elements.button.cancel.label')}
          </Button>
          <Button type="submit" form="currency-dialog-form">
            {isNew ? t('elements.button.create.label') : t('elements.button.update.label')}
          </Button>
        </div>
      }
    >
      <form
        id="currency-dialog-form"
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        {isNew ? (
          <div className="flex flex-col gap-2">
            <Label htmlFor="currency-code">{t('modules.classifications.currencies.forms.currency.code.label')}</Label>
            <Input
              id="currency-code"
              maxLength={3}
              value={code}
              onChange={(e) => setCode(e.target.value.toUpperCase())}
            />
          </div>
        ) : null}

        <div className="flex flex-col gap-2">
          <Label htmlFor="currency-name">{t('modules.classifications.currencies.forms.currency.name.label')}</Label>
          <Input id="currency-name" maxLength={64} value={name} onChange={(e) => setName(e.target.value)} />
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="currency-symbol">{t('modules.classifications.currencies.forms.currency.symbol.label')}</Label>
          <Input id="currency-symbol" maxLength={12} value={symbol} onChange={(e) => setSymbol(e.target.value)} />
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="currency-fraction-digits">{t('modules.classifications.currencies.forms.currency.fraction_digits.label')}</Label>
          <Input
            id="currency-fraction-digits"
            type="number"
            min={0}
            max={8}
            value={fractionDigits}
            onChange={(e) => setFractionDigits(Number(e.target.value))}
          />
        </div>

        {isNew ? (
          <div className="flex flex-col gap-2">
            <Label htmlFor="currency-rate">{t('modules.classifications.currencies.forms.currency.rate.label')}</Label>
            <Input id="currency-rate" value={rate} onChange={(e) => setRate(e.target.value)} />
          </div>
        ) : null}
      </form>
    </ResponsiveDialog>
  )
}
