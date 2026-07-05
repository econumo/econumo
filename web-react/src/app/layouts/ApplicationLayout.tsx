import { useRef } from 'react'
import { Link, Outlet, useLocation } from 'react-router'
import { useIsFetching, useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
// ?inline forces a data URI: the file is over vite's 4KB auto-inline cutoff,
// so without it the footer logo ships as a separate asset and can 404 where
// the header logo (under the cutoff, auto-inlined) still shows.
import grayLogo from '@/assets/econumo-gray.svg?inline'
import { LoadingDialog } from '@/components/LoadingDialog'
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
  const isFetching = useIsFetching() > 0

  const userBlock = user ? (
    <Link to={RouterPage.SETTINGS_PROFILE} className={`flex items-center gap-3 px-4 py-3 hover:bg-accent ${isCompact ? '' : 'mt-3'}`}>
      <img src={`${user.avatar}?s=100`} alt={user.name} className="size-12 rounded-xl" />
      <span className="flex min-w-0 flex-col">
        <span className="truncate text-sm font-medium">{user.name}</span>
        <span className="truncate text-xs text-muted-foreground">{user.email}</span>
      </span>
    </Link>
  ) : null

  return (
    // Fixed-height shell: the sidebar and the workspace scroll independently
    // (the window itself never scrolls), matching the Vue layout.
    <div className="flex h-svh overflow-hidden">
      {showSidebar ? (
        <aside className="flex w-full flex-col border-r bg-sidebar lg:w-80" data-testid="sidebar">
          {/* On desktop the user block stays pinned above the scrolling tree;
              on compact it scrolls away with the account list (Vue parity). */}
          {user && !isCompact ? userBlock : null}

          {isFullyLoaded || hasLoadedOnce.current ? (
            <div className="flex-1 overflow-y-auto scrollbar-none">
              {user && isCompact ? userBlock : null}
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

          <footer className="flex items-center justify-between border-t px-4 py-3">
            <div className="flex flex-col gap-2">
              <div className="flex items-center gap-0.5">
                <img src={grayLogo} width={125} height={20} alt="" />
                <span className="self-start text-[10px] text-muted-foreground">{econumoPackage().label}</span>
              </div>
              <Link to={RouterPage.SETTINGS} className="text-xs text-muted-foreground hover:text-foreground">
                {t('pages.settings.settings.menu_item')}
              </Link>
            </div>
            <button
              type="button"
              aria-label="sync"
              className="text-muted-foreground hover:text-foreground"
              onClick={() => void queryClient.invalidateQueries()}
            >
              <RefreshCw className={`size-6 ${isFetching ? 'animate-spin' : ''}`} />
            </button>
          </footer>
        </aside>
      ) : null}

      {showWorkspace ? (
        <main className="min-w-0 flex-1 overflow-y-auto" data-testid="workspace">
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
