import { ChevronRight, RefreshCw } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { ChevronLeft } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { CurrencySelect } from '@/components/CurrencySelect'
import { formatDateTime } from '@/lib/datetime'
import { getLocaleOptions } from '@/lib/config'
import { useIsCompact } from '@/hooks/useIsCompact'
import { useNavigate } from 'react-router'
import { RouterPage } from '@/app/router-pages'
import { useAccounts, useFolders } from '@/features/accounts/queries'
import { useTransactions } from '@/features/transactions/queries'
import { useCategories, usePayees, useTags } from '@/features/classifications/queries'
import { useCurrencies } from '@/features/currencies/queries'
import { useUserData, useUpdateCurrency, userCurrencyId } from '@/features/user/queries'

function MenuRow({ label, to, onClick, trailing }: { label: string; to?: string; onClick?: () => void; trailing?: React.ReactNode }) {
  const inner = (
    <span className="flex w-full items-center justify-between gap-2 rounded-md px-3 py-2.5 text-sm hover:bg-accent">
      <span>{label}</span>
      {trailing ?? <ChevronRight className="size-4 text-muted-foreground" />}
    </span>
  )
  if (to) {
    return <Link to={to}>{inner}</Link>
  }
  return (
    <button type="button" className="w-full text-left" onClick={onClick}>
      {inner}
    </button>
  )
}

export function SettingsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isCompact = useIsCompact()
  const queryClient = useQueryClient()
  const { data: user } = useUserData()
  const { data: currencies } = useCurrencies()
  const updateCurrency = useUpdateCurrency()

  // lastSyncAt = the oldest fetch among the core lists (Vue takes the min of the *LoadedAt stamps)
  const updatedAts = [useAccounts(), useFolders(), useCategories(), usePayees(), useTags(), useTransactions(), useCurrencies()]
    .map((q) => q.dataUpdatedAt)
    .filter((ts) => ts > 0)
  const lastSyncAt = updatedAts.length ? formatDateTime(new Date(Math.min(...updatedAts))) : '-'

  const currentCurrencyId = userCurrencyId(user)

  return (
    <div className="flex h-full flex-col gap-3 p-4">
      {isCompact ? (
        <header className="flex items-center gap-2">
          <Button type="button" variant="ghost" size="icon" aria-label="back" onClick={() => navigate(RouterPage.HOME)}>
            <ChevronLeft className="size-5" />
          </Button>
          <h1 className="flex-1 truncate text-lg font-semibold">{t('pages.settings.settings.header')}</h1>
        </header>
      ) : (
        <h1 className="text-xl font-semibold">{t('pages.settings.settings.header_desktop')}</h1>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto">
        {user ? (
          <Link to={RouterPage.SETTINGS_PROFILE} className="flex items-center gap-3 rounded-md px-3 py-3 hover:bg-accent">
            <img src={`${user.avatar}?s=50`} alt={user.name} className="size-10 rounded-full" />
            <span className="flex min-w-0 flex-col">
              <span className="truncate text-sm font-medium">{user.name}</span>
              <span className="truncate text-xs text-muted-foreground">{user.email}</span>
            </span>
          </Link>
        ) : null}

        <p className="px-3 pb-1 pt-4 text-xs font-medium uppercase text-muted-foreground">{t('pages.settings.settings.groups.service')}</p>
        <nav className="flex flex-col">
          <MenuRow
            label={t('pages.settings.sync.menu_item')}
            onClick={() => void queryClient.invalidateQueries()}
            trailing={
              <span className="flex items-center gap-2 text-xs text-muted-foreground">
                {lastSyncAt}
                <RefreshCw className="size-4" />
              </span>
            }
          />
          <MenuRow label={t('modules.connections.pages.settings.menu_item')} to={RouterPage.SETTINGS_CONNECTIONS} />
          <MenuRow label={t('modules.budget.page.settings.menu_item')} to={RouterPage.SETTINGS_BUDGETS} />
          <MenuRow label={t('pages.settings.accounts.menu_item')} to={RouterPage.SETTINGS_ACCOUNTS} />
          <MenuRow label={t('modules.classifications.categories.pages.settings.menu_item')} to={RouterPage.SETTINGS_CATEGORIES} />
          <MenuRow label={t('modules.classifications.payees.pages.settings.menu_item')} to={RouterPage.SETTINGS_PAYEES} />
          <MenuRow label={t('modules.classifications.tags.pages.settings.menu_item')} to={RouterPage.SETTINGS_TAGS} />
          <div className="flex items-center justify-between gap-2 px-3 py-2.5">
            <span className="text-sm">{t('pages.settings.currency.menu_item')}</span>
            <div className="w-40">
              <CurrencySelect
                aria-label={t('pages.settings.currency.menu_item')}
                value={currentCurrencyId}
                onChange={(id) => {
                  const currency = currencies?.find((c) => c.id === id)
                  if (currency) {
                    updateCurrency.mutate({ currency: currency.code })
                  }
                }}
              />
            </div>
          </div>
        </nav>

        {getLocaleOptions().length > 1 ? (
          <>
            <p className="px-3 pb-1 pt-4 text-xs font-medium uppercase text-muted-foreground">
              {t('pages.settings.settings.groups.user_interface')}
            </p>
            <MenuRow label={t('pages.settings.language.menu_item')} onClick={() => {}} />
          </>
        ) : null}
      </div>
    </div>
  )
}
