import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { EntityIcon } from '@/components/EntityIcon'
import { moneyFormat } from '@/lib/money'
import { useIsCompact } from '@/hooks/useIsCompact'
import { useSidebarStore, useUiStore } from '@/app/uiStore'
import { RouterPage } from '@/app/router-pages'
import { useAccounts, useFolders } from './queries'
import { useCurrencies } from '@/features/currencies/queries'
import { useExchange } from '@/features/currencies/useExchange'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { buildAccountsTree } from './accountsTree'
import type { AccountDto } from '@/api/dto/account'

// Vue marks shared accounts with the Material "group" icon in the desktop
// sidebar; the compact (mobile) account cards show the avatar cluster instead.
function SharedMark({ account, selected }: { account: AccountDto; selected: boolean }) {
  if (account.sharedAccess.length === 0) {
    return null
  }
  return <EntityIcon name="group" aria-label="shared" className={`text-lg ${selected ? 'text-white/80' : 'text-muted-foreground'}`} />
}

function SharedAvatars({ account }: { account: AccountDto }) {
  if (account.sharedAccess.length === 0) {
    return null
  }
  return (
    <span className="absolute right-4 top-2.5 flex" aria-label="shared">
      <img src={`${account.owner.avatar}?s=30`} alt={account.owner.name} className="-mr-2 size-7 rounded-full border-2 border-econumo-card" />
      {account.sharedAccess.map((access) => (
        <img
          key={access.user.id}
          src={`${access.user.avatar}?s=30`}
          alt={access.user.name}
          className="-mr-2 size-7 rounded-full border-2 border-econumo-card"
        />
      ))}
    </span>
  )
}

export function SidebarAccountTree() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isCompact = useIsCompact()
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
    <nav className={isCompact ? 'flex flex-col gap-3 px-4 py-3' : 'flex flex-col gap-1 px-3 py-2'}>
      {tree.map((item) => {
        const open = folderOpen[item.folder.id] ?? true
        return (
          <Collapsible key={item.folder.id} open={open} onOpenChange={() => toggleFolder(item.folder.id)}>
            {isCompact ? (
              // Vue mobile home: big folder header (name over the converted
              // total) with a square chevron button on the right.
              <CollapsibleTrigger className="flex w-full items-center gap-2 rounded-2xl py-1.5 pl-2">
                <span className="flex min-w-0 flex-1 flex-col text-left">
                  <span className="truncate text-lg" title={item.folder.name}>
                    {item.folder.name}
                  </span>
                  <span className="text-sm font-medium text-muted-foreground">
                    {item.currency ? moneyFormat(item.total, item.currency) : moneyFormat(item.total)}
                  </span>
                </span>
                <span className="grid size-12 shrink-0 place-items-center rounded-lg bg-econumo-card">
                  {open ? (
                    <ChevronDown className="size-5 text-muted-foreground" />
                  ) : (
                    <ChevronRight className="size-5 text-muted-foreground" />
                  )}
                </span>
              </CollapsibleTrigger>
            ) : (
              <CollapsibleTrigger className="flex w-full items-center gap-2 rounded-md px-2 py-2.5 text-[15px] hover:bg-accent">
                <span className="flex-1 truncate text-left" title={item.folder.name}>
                  {item.folder.name}
                </span>
                <span className="text-[13px] text-muted-foreground">
                  {item.currency ? moneyFormat(item.total, item.currency) : moneyFormat(item.total)}
                </span>
                {open ? (
                  <ChevronDown className="size-4 text-muted-foreground" />
                ) : (
                  <ChevronRight className="size-4 text-muted-foreground" />
                )}
              </CollapsibleTrigger>
            )}
            <CollapsibleContent>
              <ul className={isCompact ? 'grid grid-cols-2 gap-2 pt-2 sm:grid-cols-3' : 'flex flex-col gap-0.5'}>
                {item.accounts.map((account) => {
                  const selected = account.id === selectedAccountId
                  return (
                    <li key={account.id}>
                      {isCompact ? (
                        // Vue mobile home: account tiles — icon on top, name
                        // and balance below, shared avatars pinned top-right.
                        <button
                          type="button"
                          aria-current={selected ? 'page' : undefined}
                          className={`relative flex w-full flex-col items-start overflow-hidden rounded-2xl p-3 pb-2 text-left ${
                            selected
                              ? 'bg-gradient-to-r from-econumo-magenta to-econumo-magenta-dark text-white'
                              : 'bg-econumo-card'
                          }`}
                          onClick={() => navigate(RouterPage.ACCOUNT(account.id))}
                        >
                          <EntityIcon name={account.icon} className={`text-2xl ${selected ? 'text-white' : 'text-[#666666]'}`} />
                          <span className="mb-1 mt-2 min-h-8 w-full truncate text-sm" title={account.name}>
                            {account.name}
                          </span>
                          <span className={`text-sm font-medium ${selected ? 'text-white/85' : 'text-muted-foreground'}`}>
                            {moneyFormat(account.balance, account.currency)}
                          </span>
                          <SharedAvatars account={account} />
                        </button>
                      ) : (
                        <button
                          type="button"
                          aria-current={selected ? 'page' : undefined}
                          className={`flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left ${
                            selected
                              ? 'bg-gradient-to-r from-econumo-magenta to-econumo-magenta-dark text-white'
                              : 'hover:bg-accent'
                          }`}
                          onClick={() => navigate(RouterPage.ACCOUNT(account.id))}
                        >
                          <span
                            className={`grid size-9 shrink-0 place-items-center rounded-lg ${selected ? 'bg-white/20' : 'bg-econumo-card'}`}
                          >
                            <EntityIcon name={account.icon} className={`text-lg ${selected ? 'text-white' : 'text-[#666666]'}`} />
                          </span>
                          <span className="flex min-w-0 flex-1 flex-col">
                            <span className="truncate text-sm leading-tight" title={account.name}>
                              {account.name}
                            </span>
                            <span className={`text-[13px] leading-tight ${selected ? 'text-white/85' : 'text-muted-foreground'}`}>
                              {moneyFormat(account.balance, account.currency)}
                            </span>
                          </span>
                          <SharedMark account={account} selected={selected} />
                        </button>
                      )}
                    </li>
                  )
                })}
              </ul>
            </CollapsibleContent>
          </Collapsible>
        )
      })}
    </nav>
  )
}
