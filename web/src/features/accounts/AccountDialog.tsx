import { useEffect, useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { v7 as uuidv7 } from 'uuid'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { CalculatorInput } from '@/components/CalculatorInput'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { CurrencyPickerDialog } from '@/components/CurrencyPickerDialog'
import { IconPicker } from '@/components/IconPicker'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { formatDateTime } from '@/lib/datetime'
import { defaultAccountIcon } from '@/lib/icons'
import { moneyFormat } from '@/lib/money'
import { evaluateFormula, sanitizeInput } from '@/lib/calculator'
import { isNotEmpty, isValidAccountName, isValidDecimalNumber, isValidFormula, isValidNumber } from '@/lib/validation'
import { useUiStore } from '@/app/uiStore'
import { useCurrencies } from '@/features/currencies/queries'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { useCreateAccount, useUpdateAccount } from './queries'

export function AccountDialog() {
  const { t } = useTranslation()
  const params = useUiStore((s) => s.accountModal)
  const close = useUiStore((s) => s.closeAccountModal)
  const { data: user } = useUserData()
  const { data: currencies } = useCurrencies()
  const createAccount = useCreateAccount()
  const updateAccount = useUpdateAccount()

  const account = params?.account
  const isNew = !account

  const [name, setName] = useState('')
  const [balance, setBalance] = useState('0')
  const [currencyId, setCurrencyId] = useState<string | null>(null)
  const [currencyOpen, setCurrencyOpen] = useState(false)
  const [icon, setIcon] = useState(defaultAccountIcon)
  const [errors, setErrors] = useState<{ name?: string; balance?: string }>({})

  useEffect(() => {
    if (!params) {
      return
    }
    if (params.account) {
      setName(params.account.name)
      setBalance(
        moneyFormat(params.account.balance, params.account.currency, {
          showCurrency: false,
          useNativePrecision: false,
          useThousandSeparator: false,
        }),
      )
      setCurrencyId(params.account.currency.id)
      setIcon(params.account.icon || defaultAccountIcon)
    } else {
      setName('')
      setBalance('0')
      setCurrencyId(userCurrencyId(user))
      setIcon(defaultAccountIcon)
    }
    setErrors({})
    // re-seed whenever the dialog opens with new params
  }, [params, user])

  if (!params) {
    return null
  }

  const validate = (): boolean => {
    const next: { name?: string; balance?: string } = {}
    if (!isNotEmpty(name)) {
      next.name = t('elements.validation.required_field')
    } else if (!isValidAccountName(name)) {
      next.name = t('elements.form.account.name.validation.invalid_name')
    }
    if (!isNotEmpty(balance)) {
      next.balance = t('elements.validation.required_field')
    } else if (!isValidFormula(balance)) {
      next.balance = t('elements.validation.invalid_formula')
    } else {
      const evaluated = evaluateFormula(sanitizeInput(balance) + '=')
      if (!isValidNumber(evaluated)) {
        next.balance = t('elements.validation.invalid_number')
      } else if (!isValidDecimalNumber(evaluated)) {
        next.balance = t('elements.validation.invalid_decimal_number')
      }
    }
    setErrors(next)
    return Object.keys(next).length === 0
  }

  const submit = async () => {
    if (!validate() || !currencyId) {
      return
    }
    const numericBalance = Number(evaluateFormula(sanitizeInput(balance) + '='))
    try {
      if (isNew) {
        await createAccount.mutateAsync({
          id: uuidv7(),
          name,
          currencyId,
          balance: numericBalance,
          icon,
          folderId: params.folderId ?? null,
        })
      } else {
        await updateAccount.mutateAsync({
          id: account.id,
          name,
          balance: numericBalance,
          icon,
          currencyId,
          updatedAt: formatDateTime(new Date()),
        })
      }
      close()
    } catch {
      // keep the dialog open; field errors arrive via the envelope in later plans
    }
  }

  const pending = createAccount.isPending || updateAccount.isPending

  return (
    <ResponsiveDialog
      open
      caps
      fullScreen
      onOpenChange={(o) => !o && close()}
      title={isNew ? t('modals.account.create_form.header') : t('modals.account.update_form.header')}
      footer={
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={close}>
            {t('elements.button.cancel.label')}
          </Button>
          <Button type="submit" form="account-dialog-form" disabled={pending}>
            {isNew ? t('elements.button.add.label') : t('elements.button.update.label')}
          </Button>
        </div>
      }
    >
      <form
        id="account-dialog-form"
        // min-h-full: on the full-screen mobile page the last (icon) block grows
        // into the leftover height; desktop's auto-height dialog ignores it
        className="flex min-h-full flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          void submit()
        }}
      >
        <CardField label={t('elements.form.account.name.label')} htmlFor="account-name" error={errors.name}>
          <Input
            id="account-name"
            className={cardFieldControlClass}
            maxLength={64}
            placeholder={t('elements.form.account.name.placeholder')}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </CardField>

        <CardField label={t('elements.form.account.balance.label')} htmlFor="account-balance" error={errors.balance}>
          <CalculatorInput
            id="account-balance"
            className={cardFieldControlClass}
            placeholder={t('elements.form.account.balance.placeholder')}
            value={balance}
            onChange={setBalance}
          />
        </CardField>

        {/* same card shape, but a picker row: tap opens the currency search dialog */}
        <button
          type="button"
          className="flex w-full items-center justify-between gap-3 rounded-lg bg-econumo-card px-4 py-2.5 text-left hover:bg-econumo-hover"
          title={t('elements.form.account.currency.label')}
          onClick={() => setCurrencyOpen(true)}
        >
          <span className="flex min-w-0 flex-col gap-0.5">
            <span className="text-[11px] text-muted-foreground">{t('elements.form.account.currency.label')}</span>
            <span className="truncate text-sm">{currencies?.find((c) => c.id === currencyId)?.code ?? ''}</span>
          </span>
          <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
        </button>

        <div className="flex min-h-0 flex-1 flex-col gap-2">
          <Label>{t('modals.account.form.icon.label')}</Label>
          <IconPicker fill value={icon} onChange={setIcon} aria-label={t('modals.account.form.icon.label')} />
        </div>
      </form>

      <CurrencyPickerDialog
        open={currencyOpen}
        title={t('elements.form.account.currency.label')}
        value={currencyId}
        onClose={() => setCurrencyOpen(false)}
        onPick={setCurrencyId}
      />
    </ResponsiveDialog>
  )
}
