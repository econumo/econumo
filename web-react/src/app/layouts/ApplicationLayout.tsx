import { useRef } from 'react'
import { Link, Outlet, useLocation } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import logo from '@/assets/econumo.svg'
import grayLogo from '@/assets/econumo-gray.svg'
import { LoadingDialog } from '@/components/LoadingDialog'
import { getWebsiteUrl } from '@/lib/config'
import { econumoPackage } from '@/lib/package'
import { useIsCompact } from '@/hooks/useIsCompact'
import { RouterPage } from '@/app/router-pages'
import { SidebarAccountTree } from '@/features/accounts/SidebarAccountTree'
import { AccountDialog } from '@/features/accounts/AccountDialog'
import { SwitchAccountPrompt } from '@/features/accounts/SwitchAccountPrompt'
import { TransactionDialog } from '@/features/transactions/TransactionDialog'
import { useAccounts, useFolders } from '@/features/accounts/queries'
import { useTransactions } from '@/features/transactions/queries'
import { useCategories, usePayees, useTags } from '@/features/classifications/queries'
import { useCurrencies, useCurrencyRates } from '@/features/currencies/queries'
import { useUserData, isOnboardingCompleted } from '@/features/user/queries'
import { useBudgets } from '@/features/budgets/queries'

function useIsFullyLoaded() {
  const queries = [
    useAccounts(),
    useFolders(),
    useTransactions(),
    useCategories(),
    usePayees(),
    useTags(),
    useCurrencies(),
    useCurrencyRates(),
    useUserData(),
    useBudgets(),
  ]
  return queries.every((q) => q.data !== undefined)
}

export function ApplicationLayout() {
  const { t } = useTranslation()
  const location = useLocation()
  const queryClient = useQueryClient()
  const isCompact = useIsCompact()
  const isFullyLoaded = useIsFullyLoaded()
  const { data: user } = useUserData()

  // The blocking loader belongs to the FIRST boot only; once data has been on
  // screen, refetches and cache churn must never re-cover the app (Vue parity).
  const hasLoadedOnce = useRef(false)
  if (isFullyLoaded) {
    hasLoadedOnce.current = true
  }
  const showBootLoader = !isFullyLoaded && !hasLoadedOnce.current

  const showSidebar = !isCompact || location.pathname === '/'
  const showWorkspace = !isCompact || location.pathname !== '/'
  const websiteUrl = getWebsiteUrl()

  return (
    <div className="flex min-h-svh">
      {showSidebar ? (
        <aside className="flex w-full flex-col border-r bg-sidebar lg:w-80" data-testid="sidebar">
          <div className="flex items-center gap-2 px-4 pb-2 pt-4">
            <img src={logo} width={145} height={15} alt={t('elements.econumo.label')} />
            <span className="text-[10px] text-muted-foreground">{econumoPackage().label}</span>
          </div>
          {user ? (
            <Link to={RouterPage.SETTINGS_PROFILE} className="flex items-center gap-3 px-4 py-3 hover:bg-accent">
              <img src={`${user.avatar}?s=100`} alt={user.name} className="size-9 rounded-full" />
              <span className="flex min-w-0 flex-col">
                <span className="truncate text-sm font-medium">{user.name}</span>
                <span className="truncate text-xs text-muted-foreground">{user.email}</span>
              </span>
            </Link>
          ) : null}

          {isFullyLoaded || hasLoadedOnce.current ? (
            <div className="flex-1 overflow-y-auto">
              <div className="flex flex-col px-3 py-1">
                {!isOnboardingCompleted(user) ? (
                  <Link to={RouterPage.ONBOARDING} className="rounded-md px-2 py-2 text-[15px] hover:bg-accent">
                    {t('blocks.main.onboarding')}
                  </Link>
                ) : null}
                <Link to={RouterPage.BUDGET} className="rounded-md px-2 py-2 text-[15px] hover:bg-accent">
                  {t('blocks.main.budget')}
                </Link>
              </div>
              <SidebarAccountTree />
            </div>
          ) : (
            <div className="flex-1" />
          )}

          <footer className="flex flex-col gap-2 border-t px-4 py-3">
            <div className="flex items-center gap-2">
              <img src={grayLogo} width={100} height={10} alt="" />
              <span className="text-[10px] text-muted-foreground">{econumoPackage().label}</span>
              <button
                type="button"
                aria-label="sync"
                className="ml-auto text-muted-foreground hover:text-foreground"
                onClick={() => void queryClient.invalidateQueries()}
              >
                <RefreshCw className="size-4" />
              </button>
            </div>
            <div className="flex items-center gap-4 text-sm text-muted-foreground">
              <Link to={RouterPage.SETTINGS} className="hover:text-foreground">
                {t('pages.settings.settings.menu_item')}
              </Link>
              {websiteUrl ? (
                <a href={websiteUrl} target="_blank" rel="nofollow" aria-label={t('blocks.help.label')} className="hover:text-foreground">
                  {t('blocks.website.label')}
                </a>
              ) : null}
            </div>
          </footer>
        </aside>
      ) : null}

      {showWorkspace ? (
        <main className="min-w-0 flex-1" data-testid="workspace">
          <Outlet />
        </main>
      ) : null}

      <AccountDialog />
      <TransactionDialog />
      <SwitchAccountPrompt />
      <LoadingDialog open={showBootLoader} label={t('modules.app.modal.loading.data_loading')} />
    </div>
  )
}
