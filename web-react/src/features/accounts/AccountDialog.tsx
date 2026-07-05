import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { v7 as uuidv7 } from 'uuid'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { CalculatorInput } from '@/components/CalculatorInput'
import { CurrencySelect } from '@/components/CurrencySelect'
import { EntityIcon } from '@/components/EntityIcon'
import { IconPicker } from '@/components/IconPicker'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { formatDateTime } from '@/lib/datetime'
import { defaultAccountIcon } from '@/lib/icons'
import { moneyFormat } from '@/lib/money'
import { evaluateFormula, sanitizeInput } from '@/lib/calculator'
import { isNotEmpty, isValidAccountName, isValidDecimalNumber, isValidFormula, isValidNumber } from '@/lib/validation'
import { useUiStore } from '@/app/uiStore'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { useCreateAccount, useUpdateAccount } from './queries'

export function AccountDialog() {
  const { t } = useTranslation()
  const params = useUiStore((s) => s.accountModal)
  const close = useUiStore((s) => s.closeAccountModal)
  const { data: user } = useUserData()
  const createAccount = useCreateAccount()
  const updateAccount = useUpdateAccount()

  const account = params?.account
  const isNew = !account

  const [name, setName] = useState('')
  const [balance, setBalance] = useState('0')
  const [currencyId, setCurrencyId] = useState<string | null>(null)
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
      onOpenChange={(o) => !o && close()}
      title={isNew ? t('modals.account.create_form.header') : t('modals.account.update_form.header')}
    >
      <form
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          void submit()
        }}
      >
        <div className="flex flex-col gap-2">
          <Label htmlFor="account-name">{t('elements.form.account.name.label')}</Label>
          <Input
            id="account-name"
            maxLength={64}
            placeholder={t('elements.form.account.name.placeholder')}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          {errors.name ? <p className="text-sm text-destructive">{errors.name}</p> : null}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="account-balance">{t('elements.form.account.balance.label')}</Label>
          <CalculatorInput
            id="account-balance"
            placeholder={t('elements.form.account.balance.placeholder')}
            value={balance}
            onChange={setBalance}
          />
          {errors.balance ? <p className="text-sm text-destructive">{errors.balance}</p> : null}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="account-currency">{t('elements.form.account.currency.label')}</Label>
          <CurrencySelect id="account-currency" aria-label={t('elements.form.account.currency.label')} value={currencyId} onChange={setCurrencyId} />
        </div>

        <div className="flex flex-col gap-2">
          <Label>{t('modals.account.form.icon.label')}</Label>
          <IconPicker value={icon} onChange={setIcon} aria-label={t('modals.account.form.icon.label')} />
        </div>

        <div className="grid grid-cols-2 gap-3">
          <Button type="button" variant="secondary" onClick={close}>
            {t('elements.button.cancel.label')}
          </Button>
          <Button type="submit" disabled={pending}>
            {isNew ? t('elements.button.add.label') : t('elements.button.update.label')}
          </Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
