import { useRef } from 'react'
import { Link, Outlet, useLocation } from 'react-router'
import { useIsFetching, useIsRestoring, useQueryClient } from '@tanstack/react-query'
import { RefreshCw, Rocket, Settings, Wallet } from 'lucide-react'
import { useTranslation } from 'react-i18next'
// ?inline forces a data URI: the file is over vite's 4KB auto-inline cutoff,
// so without it the footer logo ships as a separate asset and can 404 where
// the header logo (under the cutoff, auto-inlined) still shows.
import grayLogo from '@/assets/econumo-gray.svg?inline'
import { LoadingDialog } from '@/components/LoadingDialog'
import { UserCard } from '@/components/UserCard'
import { UserAvatar } from '@/components/UserAvatar'
import { econumoPackage } from '@/lib/package'
import { formatDateTime } from '@/lib/datetime'
import { useIsCompact } from '@/hooks/useIsCompact'
import { useSidebarStore } from '@/app/uiStore'
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
import { recordPathname } from '@/lib/navigation'

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

// lastSyncAt = the oldest fetch among the core lists (Vue takes the min of the
// *LoadedAt stamps); shown in the sync button's tooltip.
function useLastSyncAt(): string {
  const stamps = [useAccounts(), useFolders(), useCategories(), usePayees(), useTags(), useTransactions(), useCurrencies()]
    .map((q) => q.dataUpdatedAt)
    .filter((ts) => ts > 0)
  return stamps.length ? formatDateTime(new Date(Math.min(...stamps))) : '-'
}

export function ApplicationLayout() {
  const { t } = useTranslation()
  const location = useLocation()
  // Recorded during render (idempotent per pathname) so children mounting in
  // this same pass can already read their origin via previousPathname().
  recordPathname(location.pathname)
  const queryClient = useQueryClient()
  const isCompact = useIsCompact()
  const isFullyLoaded = useIsFullyLoaded()
  const { data: user } = useUserData()

  // The blocking loader belongs to the FIRST boot only; once data has been on
  // screen, refetches and cache churn must never re-cover the app (Vue parity).
  // While the persisted cache is being restored the data is transiently
  // undefined — that must not flash the loader either.
  const isRestoring = useIsRestoring()
  const hasLoadedOnce = useRef(false)
  if (isFullyLoaded) {
    hasLoadedOnce.current = true
  }
  const showBootLoader = !isFullyLoaded && !hasLoadedOnce.current && !isRestoring

  const showSidebar = !isCompact || location.pathname === '/'
  const showWorkspace = !isCompact || location.pathname !== '/'
  const isFetching = useIsFetching() > 0
  const lastSyncAt = useLastSyncAt()
  const { collapsed, toggleCollapsed } = useSidebarStore()
  // Icon-rail mode is desktop-only; compact keeps the full-width home sidebar.
  const rail = collapsed && !isCompact

  const userBlock = user ? (
    rail ? (
      <Link to={RouterPage.SETTINGS_PROFILE} className="mt-3 flex justify-center px-2 py-3" title={user.name}>
        <UserAvatar avatar={user.avatar} size="md" className="rounded-xl" />
      </Link>
    ) : (
      <Link to={RouterPage.SETTINGS_PROFILE} className={`flex px-4 py-4 hover:bg-accent ${isCompact ? '' : 'mt-3'}`}>
        <UserCard user={user} />
      </Link>
    )
  ) : null

  return (
    // Fixed-height shell: the sidebar and the workspace scroll independently
    // (the window itself never scrolls), matching the Vue layout.
    // The PWA viewport is edge-to-edge (viewport-fit=cover), so the shell keeps
    // itself clear of the status bar / rounded corners; the bottom inset is
    // handled per bottom bar so their backgrounds still reach the screen edge.
    <div className="flex h-svh overflow-hidden pt-[env(safe-area-inset-top)] pr-[env(safe-area-inset-right)] pl-[env(safe-area-inset-left)]">
      {showSidebar ? (
        <aside className={`flex w-full flex-col bg-sidebar ${rail ? 'lg:w-16' : 'lg:w-80'}`} data-testid="sidebar">
          {/* On desktop the user block stays pinned above the scrolling tree;
              on compact it scrolls away with the account list (Vue parity). */}
          {user && !isCompact ? userBlock : null}

          {isFullyLoaded || hasLoadedOnce.current ? (
            <div className="flex-1 overflow-y-auto scrollbar-none">
              {user && isCompact ? userBlock : null}
              {rail ? (
                <div className="flex flex-col items-center gap-1 py-1">
                  {!isOnboardingCompleted(user) ? (
                    <Link
                      to={RouterPage.ONBOARDING}
                      title={t('blocks.main.onboarding')}
                      className="grid size-10 place-items-center rounded-lg text-muted-foreground hover:bg-accent"
                    >
                      <Rocket className="size-5" />
                    </Link>
                  ) : null}
                  <Link
                    to={RouterPage.BUDGET}
                    title={t('blocks.main.budget')}
                    className="grid size-10 place-items-center rounded-lg text-muted-foreground hover:bg-accent"
                  >
                    <Wallet className="size-5" />
                  </Link>
                </div>
              ) : (
                // compact matches the folder headers below (size and left edge)
                <div className={`flex flex-col py-1 ${isCompact ? 'px-4' : 'px-3'}`}>
                  {!isOnboardingCompleted(user) ? (
                    <Link to={RouterPage.ONBOARDING} className={`rounded-md px-2 py-2 hover:bg-accent ${isCompact ? 'text-lg' : 'text-[15px]'}`}>
                      {t('blocks.main.onboarding')}
                    </Link>
                  ) : null}
                  <Link to={RouterPage.BUDGET} className={`rounded-md px-2 py-2 hover:bg-accent ${isCompact ? 'text-lg' : 'text-[15px]'}`}>
                    {t('blocks.main.budget')}
                  </Link>
                </div>
              )}
              <SidebarAccountTree collapsed={rail} />
            </div>
          ) : (
            <div className="flex-1" />
          )}

          {rail ? (
            <footer className="flex flex-col items-center gap-4 border-t px-2 pt-3 pb-[max(env(safe-area-inset-bottom),0.75rem)]">
              <Link
                to={RouterPage.SETTINGS}
                title={t('pages.settings.settings.menu_item')}
                className="text-muted-foreground hover:text-foreground"
              >
                <Settings className="size-5" />
              </Link>
              <button
                type="button"
                aria-label="sync"
                title={`${t('pages.settings.sync.menu_item')} — ${lastSyncAt}`}
                className="text-muted-foreground hover:text-foreground"
                onClick={() => void queryClient.invalidateQueries()}
              >
                <RefreshCw className={`size-5 ${isFetching ? 'animate-spin' : ''}`} />
              </button>
            </footer>
          ) : (
            <footer className="flex items-center justify-between border-t px-4 pt-3 pb-[max(env(safe-area-inset-bottom),0.75rem)]">
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
                title={`${t('pages.settings.sync.menu_item')} — ${lastSyncAt}`}
                className="text-muted-foreground hover:text-foreground"
                onClick={() => void queryClient.invalidateQueries()}
              >
                <RefreshCw className={`size-6 ${isFetching ? 'animate-spin' : ''}`} />
              </button>
            </footer>
          )}
        </aside>
      ) : null}

      {/* The sidebar/workspace divider doubles as the collapse toggle (desktop only). */}
      {showSidebar && !isCompact ? (
        <button
          type="button"
          aria-label="toggle sidebar"
          title={t(collapsed ? 'blocks.main.expand_menu' : 'blocks.main.collapse_menu')}
          className="w-1.5 shrink-0 cursor-col-resize border-l bg-transparent p-0 hover:bg-accent"
          onClick={toggleCollapsed}
        />
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
