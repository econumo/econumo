import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { EntityIcon } from '@/components/EntityIcon'
import { UserAvatar } from '@/components/UserAvatar'
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
}

// Presentational only: the row wrapper on the page owns the click (menu on
// desktop, preview sheet on mobile) so hover/active feedback covers the whole
// row including the kebab.
export function TransactionRow({ transaction: tx, pageAccount }: TransactionRowProps) {
  const { t } = useTranslation()
  const title = transactionTitleInfo(tx, pageAccount.id, t)
  const income = isIncomeForAccount(tx, pageAccount.id)
  const icon = tx.type === 'transfer' ? 'sync_alt' : tx.category?.icon || 'question_mark'

  return (
    // Vue reference layout: 40px icon, 16px title with the amount on the same
    // top line, then description / tag / payee stacked one per line below.
    <div
      data-testid={`tx-${tx.id}`}
      className={`flex w-full items-start gap-4 px-2 py-2 text-left ${tx.isInFuture ? 'opacity-50' : ''}`}
    >
      <span className="relative grid size-10 shrink-0 place-items-center rounded-full bg-econumo-card">
        <EntityIcon name={icon} className="text-xl text-[#666666]" />
        {pageAccount.sharedAccess.length > 0 && tx.author ? (
          // the tooltip is the only place the row names the author
          <span title={tx.author.name} className="absolute -bottom-1 -right-2">
            <UserAvatar avatar={tx.author.avatar} size="xs" className="border-2 border-background" />
          </span>
        ) : null}
      </span>
      <span className="flex min-w-0 flex-1 flex-col gap-1">
        <span className="truncate text-base leading-6" title={title.text}>
          {title.text}
        </span>
        {title.source !== 'description' && tx.description ? (
          <span className="break-words text-sm text-muted-foreground">{tx.description}</span>
        ) : null}
        {title.source !== 'tag' && tx.tag ? (
          <span className="flex">
            <Badge variant="secondary" className="max-w-full" title={tx.tag.name}>
              <span className="truncate">{tx.tag.name}</span>
            </Badge>
          </span>
        ) : null}
        {title.source !== 'payee' && tx.payee ? (
          <span className="truncate text-[13px] text-muted-foreground" title={tx.payee.name}>
            {tx.payee.name}
          </span>
        ) : null}
      </span>
      <span className={`text-sm leading-6 tabular-nums ${income ? 'text-income' : 'text-expense'}`}>
        {displayAmount(tx, pageAccount.id)}
        <span className="ml-1 text-muted-foreground">{pageAccount.currency.symbol}</span>
      </span>
    </div>
  )
}
