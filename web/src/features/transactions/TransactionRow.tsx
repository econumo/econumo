import { ArrowDownToLine, Repeat } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { EntityIcon } from '@/components/EntityIcon'
import { UserAvatar } from '@/components/UserAvatar'
import { moneyFormat } from '@/lib/money'
import type { AccountDto } from '@/api/dto/account'
import type { Id } from '@/api/types'
import { isIncomeForAccount, transactionTitleInfo } from './useAccountTransactions'
import type { TitleInfo, ViewTransaction } from './useAccountTransactions'

// Amount text per the Vue transactionDisplayAmount: sign prepended manually,
// number formatted with the TRANSACTION account's currency, while the trailing
// symbol is the PAGE account's (a Vue quirk kept for parity).
export function displayAmount(tx: ViewTransaction, pageAccountId?: Id): string {
  const opts = { showCurrency: false, useNativePrecision: false } as const
  if (tx.type === 'transfer') {
    // global mode: a transfer is not income or expense of any one account
    if (!pageAccountId) {
      // a hidden source account leaves only the received side to show
      return tx.account
        ? moneyFormat(tx.amount, tx.account.currency, opts)
        : moneyFormat(tx.amountRecipient ?? tx.amount, tx.accountRecipient?.currency, opts)
    }
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
  /** omit for the cross-account (global search) list: the row then names its
      account and treats a transfer as neutral */
  pageAccount?: AccountDto
  /** Dim the row to mark money that hasn't moved yet — a future-dated
      transaction or an unposted template preview. The account list leaves this
      to the default; the recurring settings list turns it off, since there
      EVERY row is a template and dimming them all would just look disabled. */
  dimmed?: boolean
  /** muted note trailing the title (the recurring list's "Monthly") — kept
      outside the truncating box so a long title can never clip it */
  titleNote?: string
  /** small muted line under the amount (the recurring list's next payment) */
  amountNote?: React.ReactNode
  /** overrides the default author-avatar rule (shown only on shared accounts) */
  showAuthor?: boolean
}

function AccountName({ account, fallback }: { account?: AccountDto; fallback: string }) {
  return (
    <span className="flex min-w-0 items-center gap-1" title={account?.name ?? fallback}>
      <EntityIcon name={account?.icon || 'account_balance_wallet'} className="shrink-0 text-sm" />
      <span className="truncate">{account?.name ?? fallback}</span>
    </span>
  )
}

// Presentational only: the row wrapper on the page owns the click (menu on
// desktop, preview sheet on mobile) so hover/active feedback covers the whole
// row including the kebab.
export function TransactionRow({ transaction: tx, pageAccount, dimmed, titleNote, amountNote, showAuthor: showAuthorProp }: TransactionRowProps) {
  const { t } = useTranslation()
  const global = !pageAccount
  const title: TitleInfo =
    global && tx.type === 'transfer'
      ? {
          text: tx.description || t('transactions.modal.transaction_type.transfer'),
          source: tx.description ? 'description' : 'transfer',
        }
      : transactionTitleInfo(tx, pageAccount?.id ?? tx.accountId, t)
  const income = isIncomeForAccount(tx, pageAccount?.id ?? tx.accountId)
  const neutral = global && tx.type === 'transfer'
  const amountAccount = pageAccount ?? tx.account ?? tx.accountRecipient
  const showAuthor = showAuthorProp ?? (global ? Boolean(tx.account?.sharedAccess.length) : (pageAccount?.sharedAccess.length ?? 0) > 0)
  const hiddenName = t('accounts.account.name_hidden')
  const icon = tx.type === 'transfer' ? 'sync_alt' : tx.category?.icon || 'question_mark'
  // The glyph marks "this is on a schedule" either way: an unposted preview
  // (tx.recurring) or a real transaction posted from a template (recurringId).
  // Dimming stays exclusive to the preview — a posted transaction is settled
  // money and must read as solid as any hand-entered row.
  const isRecurring = Boolean(tx.recurring) || Boolean(tx.recurringId)

  return (
    // Vue reference layout: 40px icon, 16px title with the amount on the same
    // top line, then description / tag / payee stacked one per line below.
    <div
      data-testid={`tx-${tx.id}`}
      /* tx.recurring (not isRecurring) on purpose: only the unposted preview is
         dimmed, never a posted transaction */
      className={`flex w-full items-start gap-4 px-2 py-2 text-left ${(dimmed ?? (tx.isInFuture || Boolean(tx.recurring))) ? 'opacity-50' : ''}`}
    >
      <span className="relative grid size-10 shrink-0 place-items-center rounded-full bg-econumo-card">
        <EntityIcon name={icon} className="text-xl text-[#666666]" />
        {showAuthor && tx.author ? (
          // the tooltip is the only place the row names the author
          <span title={tx.author.name} className="absolute -bottom-1 -right-2">
            <UserAvatar avatar={tx.author.avatar} size="xs" className="border-2 border-background" />
          </span>
        ) : null}
      </span>
      <span className="flex min-w-0 flex-1 flex-col gap-1">
        {/* the icon trails the name but must survive truncation, so the name
            truncates inside its own box and the icon stays a shrink-0 sibling */}
        <span className="flex min-w-0 items-center text-base leading-6">
          <span className="truncate" title={title.text}>
            {title.text}
          </span>
          {isRecurring ? (
            // the wrapper carries the tooltip: lucide icons take no title prop
            <span title={t('recurring.preview.header')} className="ml-1 flex shrink-0 items-center">
              <Repeat className="size-3 text-muted-foreground" aria-label={t('recurring.preview.header')} />
            </span>
          ) : null}
          {tx.isImported === 1 ? (
            <span title={t('imports.badge')} className="ml-1 flex shrink-0 items-center">
              <ArrowDownToLine className="size-3 text-muted-foreground" aria-label={t('imports.badge')} />
            </span>
          ) : null}
          {titleNote ? <span className="ml-1.5 shrink-0 text-[13px] text-muted-foreground">{titleNote}</span> : null}
        </span>
        {title.source !== 'description' && tx.description ? (
          <span className="break-words text-sm text-muted-foreground">{tx.description}</span>
        ) : null}
        {title.source !== 'payee' && tx.payee ? (
          <span className="truncate text-[13px] text-muted-foreground" title={tx.payee.name}>
            {tx.payee.name}
          </span>
        ) : null}
        {(title.source !== 'tag' && tx.tag) || tx.labels?.length ? (
          // one wrapping line of icon+name pairs; the icon alone tells the two
          // kinds apart, so the row stays as quiet as the payee line above it
          <span className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[13px] text-muted-foreground">
            {title.source !== 'tag' && tx.tag ? (
              <span className="flex min-w-0 items-center gap-1" title={tx.tag.name}>
                <EntityIcon name={tx.tag.icon || 'tag'} className="shrink-0 text-sm" />
                <span className="truncate">{tx.tag.name}</span>
              </span>
            ) : null}
            {tx.labels?.map((label) => (
              <span key={label.id} className="flex min-w-0 items-center gap-1" title={label.name}>
                <EntityIcon name={label.icon || 'label'} className="shrink-0 text-sm" />
                <span className="truncate">{label.name}</span>
              </span>
            ))}
          </span>
        ) : null}
      </span>
      {/* global rows name the account here, on the title's line above the
          amount; capped so a long name truncates instead of squeezing the title */}
      <span data-testid={`amount-col-${tx.id}`} className={`flex shrink-0 flex-col items-end ${global ? 'max-w-[45%]' : ''}`}>
        {global ? (
          <span data-testid={`tx-account-${tx.id}`} className="flex h-6 max-w-full items-center gap-1 text-[13px] text-muted-foreground">
            <AccountName account={tx.account} fallback={hiddenName} />
            {tx.type === 'transfer' ? (
              <>
                <span className="shrink-0">→</span>
                <AccountName account={tx.accountRecipient} fallback={hiddenName} />
              </>
            ) : null}
          </span>
        ) : null}
        <span className={`text-sm leading-6 tabular-nums ${neutral ? 'text-muted-foreground' : income ? 'text-income' : 'text-expense'}`}>
          {displayAmount(tx, pageAccount?.id)}
          <span className="ml-1 text-muted-foreground">{amountAccount?.currency.symbol}</span>
        </span>
        {amountNote ? <span className="text-xs text-muted-foreground">{amountNote}</span> : null}
      </span>
    </div>
  )
}
