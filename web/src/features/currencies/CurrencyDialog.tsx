import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { InputGroup, InputGroupAddon, InputGroupInput, InputGroupText } from '@/components/ui/input-group'
import { Label } from '@/components/ui/label'
import { Slider } from '@/components/ui/slider'
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
  /** the currency's current fixed rate, prefilled in edit mode */
  currentRate?: string
  /** the instance base currency code, framing the rate equation */
  baseCode?: string
  onClose: () => void
  onSubmit: (form: CurrencyDialogForm) => void
}

export function CurrencyDialog({ open, currency, currentRate, baseCode, onClose, onSubmit }: CurrencyDialogProps) {
  const { t } = useTranslation()
  const isNew = !currency
  const [code, setCode] = useState('')
  const [name, setName] = useState('')
  const [symbol, setSymbol] = useState('')
  const [fractionDigits, setFractionDigits] = useState(2)
  const [rate, setRate] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [rateError, setRateError] = useState<string | null>(null)

  useEffect(() => {
    if (open) {
      setCode(currency?.code ?? '')
      setName(currency?.name ?? '')
      setSymbol(currency?.symbol ?? '')
      setFractionDigits(currency?.fractionDigits ?? 2)
      setRate(currency ? (currentRate ?? '') : '')
      setError(null)
      setRateError(null)
    }
  }, [open, currency, currentRate])

  const submit = () => {
    if (!isNotEmpty(name)) {
      setError(t('classifications.currencies.forms.currency.name.validation.required_field'))
      return
    }
    // the fixed rate is mandatory: a custom currency cannot exist without one
    if (!isNotEmpty(rate)) {
      setRateError(t('common.validation.required_field'))
      return
    }
    onSubmit({ code, name, symbol, fractionDigits, rate })
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={isNew ? t('classifications.currencies.modals.create.header') : t('classifications.currencies.modals.edit.header')}
      footer={
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" form="currency-dialog-form">
            {isNew ? t('common.button.create.label') : t('common.button.update.label')}
          </Button>
        </div>
      }
    >
      <form
        id="currency-dialog-form"
        className="flex flex-col gap-3"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div className="flex flex-col gap-2">
          <Label htmlFor="currency-name">{t('classifications.currencies.forms.currency.name.label')}</Label>
          <Input id="currency-name" maxLength={64} value={name} onChange={(e) => setName(e.target.value)} />
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </div>

        <div className="grid grid-cols-5 gap-3">
          {isNew ? (
            <div className="col-span-3 flex flex-col gap-2">
              <Label htmlFor="currency-code">{t('classifications.currencies.forms.currency.code.label')}</Label>
              <Input
                id="currency-code"
                maxLength={3}
                placeholder="PTS"
                value={code}
                onChange={(e) => setCode(e.target.value.toUpperCase())}
              />
            </div>
          ) : null}
          <div className="col-span-2 flex flex-col gap-2">
            <Label htmlFor="currency-symbol">
              {t('classifications.currencies.forms.currency.symbol.label')}
              <span className="font-normal text-muted-foreground"> ({t('classifications.currencies.forms.currency.symbol.optional')})</span>
            </Label>
            <Input
              id="currency-symbol"
              maxLength={12}
              placeholder={code || currency?.code || ''}
              value={symbol}
              onChange={(e) => setSymbol(e.target.value)}
            />
          </div>
        </div>

        <div className="flex flex-col gap-2">
          <div className="flex items-center justify-between">
            <Label htmlFor="currency-fraction-digits">{t('classifications.currencies.forms.currency.fraction_digits.label')}</Label>
            <span className="text-sm text-muted-foreground">{fractionDigits}</span>
          </div>
          <Slider
            ticks
            id="currency-fraction-digits"
            aria-label={t('classifications.currencies.forms.currency.fraction_digits.label')}
            min={0}
            max={8}
            step={1}
            value={[fractionDigits]}
            onValueChange={([v]) => setFractionDigits(v)}
          />
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="currency-rate">{t('classifications.currencies.forms.currency.rate.label')}</Label>
          <InputGroup>
            {baseCode ? (
              <InputGroupAddon align="inline-start">
                <InputGroupText className="whitespace-nowrap">1 {baseCode} =</InputGroupText>
              </InputGroupAddon>
            ) : null}
            <InputGroupInput id="currency-rate" inputMode="decimal" value={rate} onChange={(e) => setRate(e.target.value)} />
            {code || currency?.code ? (
              <InputGroupAddon align="inline-end">
                <InputGroupText>{code || currency?.code}</InputGroupText>
              </InputGroupAddon>
            ) : null}
          </InputGroup>
          {rateError ? <p className="text-sm text-destructive">{rateError}</p> : null}
        </div>
      </form>
    </ResponsiveDialog>
  )
}
