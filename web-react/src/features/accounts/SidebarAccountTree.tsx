import { ChevronDown, ChevronRight, Users } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { EntityIcon } from '@/components/EntityIcon'
import { moneyFormat } from '@/lib/money'
import { useSidebarStore, useUiStore } from '@/app/uiStore'
import { RouterPage } from '@/app/router-pages'
import { useAccounts, useFolders } from './queries'
import { useCurrencies } from '@/features/currencies/queries'
import { useExchange } from '@/features/currencies/useExchange'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { buildAccountsTree } from './accountsTree'
import type { AccountDto } from '@/api/dto/account'

function SharedAvatars({ account }: { account: AccountDto }) {
  if (account.sharedAccess.length === 0) {
    return null
  }
  return (
    <span className="flex items-center gap-0.5">
      <Users className="size-3 text-muted-foreground" aria-label="shared" />
      <img src={`${account.owner.avatar}?s=30`} alt={account.owner.name} className="size-4 rounded-full" />
      {account.sharedAccess.map((access) => (
        <img key={access.user.id} src={`${access.user.avatar}?s=30`} alt={access.user.name} className="size-4 rounded-full" />
      ))}
    </span>
  )
}

export function SidebarAccountTree() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id: selectedAccountId } = useParams()
  const { data: accounts } = useAccounts()
  const { data: folders } = useFolders()
  const { data: currencies } = useCurrencies()
  const { data: user } = useUserData()
  const exchangeFn = useExchange()
  const { folderOpen, toggleFolder } = useSidebarStore()
  const openAccountModal = useUiStore((s) => s.openAccountModal)

  if (!accounts || !folders) {
    return null
  }

  const userCurrency = currencies?.find((c) => c.id === userCurrencyId(user)) ?? null
  const tree = buildAccountsTree(accounts, folders, userCurrency, exchangeFn, t('elements.folders.default_folder'))

  if (tree.length === 0) {
    return (
      <div className="px-3 py-2">
        <div className="flex items-center justify-between py-1 text-sm font-medium">
          <span>{folders[0]?.name ?? t('elements.folders.default_folder')}</span>
          <span className="text-muted-foreground">{userCurrency ? moneyFormat(0, userCurrency) : '0'}</span>
        </div>
        <button
          type="button"
          className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-accent"
          onClick={() => openAccountModal({ folderId: folders[0]?.id ?? null })}
        >
          <EntityIcon name="add_card" className="text-base" />
          <span className="flex-1 truncate text-left">{t('blocks.main.create_account')}</span>
          <span className="text-muted-foreground">0</span>
        </button>
      </div>
    )
  }

  return (
    <nav className="flex flex-col gap-1 px-3 py-2">
      {tree.map((item) => {
        const open = folderOpen[item.folder.id] ?? true
        return (
          <Collapsible key={item.folder.id} open={open} onOpenChange={() => toggleFolder(item.folder.id)}>
            <CollapsibleTrigger className="flex w-full items-center gap-1 rounded-md px-2 py-1.5 text-sm font-medium hover:bg-accent">
              {open ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
              <span className="flex-1 truncate text-left" title={item.folder.name}>
                {item.folder.name}
              </span>
              <span className="text-xs text-muted-foreground">
                {item.currency ? moneyFormat(item.total, item.currency) : moneyFormat(item.total)}
              </span>
            </CollapsibleTrigger>
            <CollapsibleContent>
              <ul>
                {item.accounts.map((account) => (
                  <li key={account.id}>
                    <button
                      type="button"
                      aria-current={account.id === selectedAccountId ? 'page' : undefined}
                      className={`flex w-full items-center gap-2 rounded-md py-1.5 pl-6 pr-2 text-sm hover:bg-accent ${account.id === selectedAccountId ? 'bg-accent font-medium' : ''}`}
                      onClick={() => navigate(RouterPage.ACCOUNT(account.id))}
                    >
                      <EntityIcon name={account.icon} className="text-base text-muted-foreground" />
                      <span className="flex-1 truncate text-left" title={account.name}>
                        {account.name}
                      </span>
                      <SharedAvatars account={account} />
                      <span className="text-xs text-muted-foreground">{moneyFormat(account.balance, account.currency)}</span>
                    </button>
                  </li>
                ))}
              </ul>
            </CollapsibleContent>
          </Collapsible>
        )
      })}
    </nav>
  )
}
