import { useTranslation } from 'react-i18next'
import { CommandItem } from '@/components/ui/command'
import { EntityIcon } from '@/components/EntityIcon'
import { UserAvatar } from '@/components/UserAvatar'
import { moneyFormat } from '@/lib/money'
import type { ClassificationType } from '@/lib/search'
import type { AccountDto } from '@/api/dto/account'
import type { CategoryDto } from '@/api/dto/category'
import type { LabelDto } from '@/api/dto/label'
import type { PayeeDto } from '@/api/dto/payee'
import type { TagDto } from '@/api/dto/tag'
import { AccountActionsMenu } from '@/features/accounts/AccountActions'
import { ClassificationActionsMenu } from '@/features/classifications/ClassificationActions'
import { TransactionRow } from '@/features/transactions/TransactionRow'
import type { ViewTransaction } from '@/features/transactions/useAccountTransactions'

export type ClassificationItem = CategoryDto | PayeeDto | TagDto | LabelDto

// rows carry their own trailing ⋯, so cmdk's checkmark slot would only add a gap
const NO_CHECK = '[&>svg:last-child]:hidden'

const PLURAL: Record<ClassificationType, 'categories' | 'payees' | 'tags' | 'labels'> = {
  category: 'categories',
  payee: 'payees',
  tag: 'tags',
  label: 'labels',
}

function classificationIcon(type: ClassificationType, item: ClassificationItem): string {
  switch (type) {
    case 'payee':
      return 'storefront'
    case 'tag':
      return (item as TagDto).icon || 'tag'
    case 'label':
      return (item as LabelDto).icon || 'label'
    case 'category':
      return (item as CategoryDto).icon
  }
}

export function AccountResult({ account, folderName, onSelect }: { account: AccountDto; folderName?: string; onSelect: () => void }) {
  return (
    <CommandItem value={`account:${account.id}`} onSelect={onSelect} data-testid={`search-account-${account.id}`} className={NO_CHECK}>
      <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-econumo-card">
        <EntityIcon name={account.icon} className="text-lg text-[#666666]" />
      </span>
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate">{account.name}</span>
        <span className="truncate text-xs text-muted-foreground">
          {moneyFormat(account.balance, account.currency)}
          {folderName ? ` · ${folderName}` : ''}
        </span>
      </span>
      {account.sharedAccess.length > 0 ? (
        <span className="flex items-center -space-x-1">
          <span title={account.owner.name}>
            <UserAvatar avatar={account.owner.avatar} size="xs" className="ring-2 ring-background" />
          </span>
          {account.sharedAccess.map((entry) => (
            <span key={entry.user.id} title={entry.user.name}>
              <UserAvatar avatar={entry.user.avatar} size="xs" className="ring-2 ring-background" />
            </span>
          ))}
        </span>
      ) : null}
      <AccountActionsMenu account={account} />
    </CommandItem>
  )
}

export function ClassificationResult({ type, item, onSelect }: { type: ClassificationType; item: ClassificationItem; onSelect: () => void }) {
  const { t } = useTranslation()
  const archived = item.isArchived === 1
  return (
    <CommandItem
      value={`${type}:${item.id}`}
      onSelect={onSelect}
      data-testid={`search-${type}-${item.id}`}
      className={`${NO_CHECK} ${archived ? 'opacity-60' : ''}`}
    >
      <span className="grid size-9 shrink-0 place-items-center">
        <EntityIcon name={classificationIcon(type, item)} className="text-lg text-muted-foreground" />
      </span>
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate">{item.name}</span>
        {archived ? (
          <span className="text-xs text-muted-foreground">{t(`classifications.${PLURAL[type]}.pages.settings.archived_item`)}</span>
        ) : null}
      </span>
      <ClassificationActionsMenu type={type} item={item} />
    </CommandItem>
  )
}

export function TransactionResult({ transaction, onSelect }: { transaction: ViewTransaction; onSelect: () => void }) {
  return (
    <CommandItem value={`tx:${transaction.id}`} onSelect={onSelect} className={`${NO_CHECK} items-stretch p-0 max-md:p-0`}>
      <div className="min-w-0 flex-1">
        <TransactionRow transaction={transaction} />
      </div>
    </CommandItem>
  )
}
