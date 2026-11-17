import { useTranslation } from 'react-i18next'
import { useAccounts } from '@/features/accounts/queries'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { defaultAccountName, writableAccounts, type StepBodyProps } from '../wizard'
import { StepTitle } from './StepTitle'

const NEW = 'new'

export function AccountStep({ state, setAnswers }: StepBodyProps) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const title = t('transactions.import_wizard.account.title')
  const { account } = state.answers
  const value = account === null || account.kind === 'column' ? '' : account.kind === 'new' ? NEW : account.accountId
  const choose = (picked: string) =>
    setAnswers('account', {
      account:
        picked === NEW
          ? { kind: 'new', name: defaultAccountName(state.fileName), currencyId: userCurrencyId(user) ?? '' }
          : { kind: 'existing', accountId: picked },
    })
  return (
    <div className="flex flex-col gap-2">
      <StepTitle>{title}</StepTitle>
      <select aria-label={title} className="h-9 w-full rounded-md border bg-transparent px-2 text-sm" value={value} onChange={(e) => choose(e.target.value)}>
        <option value="" disabled>
          {t('transactions.import_wizard.none')}
        </option>
        <option value={NEW}>{t('transactions.import_wizard.account.new')}</option>
        {writableAccounts(accounts, user?.id).map((a) => (
          <option key={a.id} value={a.id}>
            {a.name}
          </option>
        ))}
      </select>
    </div>
  )
}
