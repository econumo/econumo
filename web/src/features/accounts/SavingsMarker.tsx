import { useTranslation } from 'react-i18next'
import { isSavingsAccount, type AccountDto } from '@/api/dto/account'

export function SavingsMarker({ account, selected = false }: { account: AccountDto; selected?: boolean }) {
  const { t } = useTranslation()
  if (!isSavingsAccount(account)) {
    return null
  }
  return (
    <span className={`shrink-0 text-[11px] leading-tight ${selected ? 'text-white/80' : 'text-muted-foreground'}`}>
      {t('accounts.account.savings_marker')}
    </span>
  )
}
