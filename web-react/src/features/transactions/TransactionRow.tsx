import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { EntityIcon } from '@/components/EntityIcon'
import { moneyFormat } from '@/lib/money'
import type { AccountDto } from '@/api/dto/account'
import type { Id } from '@/api/types'
import { isIncomeForAccount, transactionTitleInfo } from './useAccountTransactions'
import type { ViewTransaction } from './useAccountTransactions'

// Amount text per the Vue transactionDisplayAmount: sign prepended manually,
// number formatted with the TRANSACTION account's currency, while the trailing
// symbol is the PAGE account's (a Vue quirk kept for parity).
export function displayAmount(tx: ViewTransaction, pageAccountId: Id): string {
  const opts = { showCurrency: false, useNativePrecision: false } as const
  if (tx.type === 'transfer') {
    if (tx.accountId === pageAccountId) {
      return '-' + moneyFormat(tx.amount, tx.account?.currency, opts)
    }
    return '+' + moneyFormat(tx.amountRecipient ?? tx.amount, tx.accountRecipient?.currency, opts)
  }
  const sign = tx.type === 'expense' ? '-' : '+'
  return sign + moneyFormat(tx.amount, tx.account?.currency, opts)
}

interface TransactionRowProps {
  transaction: ViewTransaction
  pageAccount: AccountDto
  onClick: () => void
}

export function TransactionRow({ transaction: tx, pageAccount, onClick }: TransactionRowProps) {
  const { t } = useTranslation()
  const title = transactionTitleInfo(tx, pageAccount.id, t)
  const income = isIncomeForAccount(tx, pageAccount.id)
  const icon = tx.type === 'transfer' ? 'sync_alt' : tx.category?.icon || 'question_mark'

  return (
    <button
      type="button"
      data-testid={`tx-${tx.id}`}
      className={`flex w-full items-center gap-3 rounded-md px-2 py-2 text-left hover:bg-accent ${tx.isInFuture ? 'opacity-50' : ''}`}
      onClick={onClick}
    >
      <EntityIcon name={icon} className="text-xl text-muted-foreground" />
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-sm">{title.text}</span>
        <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
          {title.source !== 'description' && tx.description ? <span className="truncate">{tx.description}</span> : null}
          {title.source !== 'tag' && tx.tag ? <Badge variant="secondary">{tx.tag.name}</Badge> : null}
          {title.source !== 'payee' && tx.payee ? <span className="truncate">{tx.payee.name}</span> : null}
        </span>
      </span>
      {pageAccount.sharedAccess.length > 0 && tx.author ? (
        <img src={`${tx.author.avatar}?s=30`} alt={tx.author.name} className="size-4 rounded-full" />
      ) : null}
      <span className={`text-sm tabular-nums ${income ? 'text-green-600' : 'text-red-600'}`}>
        {displayAmount(tx, pageAccount.id)}
        <span className="ml-1 text-muted-foreground">{pageAccount.currency.symbol}</span>
      </span>
    </button>
  )
}
