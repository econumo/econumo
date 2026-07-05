import { useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { ChevronLeft } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { CurrencySelect } from '@/components/CurrencySelect'
import { getLocaleOptions } from '@/lib/config'
import { useIsCompact } from '@/hooks/useIsCompact'
import { useNavigate } from 'react-router'
import { RouterPage } from '@/app/router-pages'
import { useCurrencies } from '@/features/currencies/queries'
import { useUserData, useUpdateCurrency, userCurrencyId } from '@/features/user/queries'
import { ExportCsvDialog } from '@/features/transactions/ExportCsvDialog'
import { ImportCsvDialog } from '@/features/transactions/ImportCsvDialog'
import { ImportResultDialog } from '@/features/transactions/ImportResultDialog'
import type { AggregatedImportResult } from '@/features/transactions/importCsv'

function MenuGroup({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <>
      <p className="px-1 pt-2 text-xs uppercase text-muted-foreground">{label}</p>
      <nav className="flex flex-col gap-2">{children}</nav>
    </>
  )
}

// Vue renders the hub as light-gray card rows in a narrow column
function MenuRow({ label, to, onClick, trailing }: { label: string; to?: string; onClick?: () => void; trailing?: React.ReactNode }) {
  const inner = (
    <span className="flex w-full items-center justify-between gap-2 rounded-lg bg-econumo-card px-4 py-3.5 text-sm hover:bg-econumo-hover">
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
  const { data: user } = useUserData()
  const { data: currencies } = useCurrencies()
  const updateCurrency = useUpdateCurrency()
  const [exportOpen, setExportOpen] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [importResult, setImportResult] = useState<AggregatedImportResult | null>(null)

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
        <div className="flex max-w-md flex-col gap-2">
          {user ? (
            <Link
              to={RouterPage.SETTINGS_PROFILE}
              className="flex items-center gap-3 rounded-lg bg-econumo-card px-4 py-3 hover:bg-econumo-hover"
            >
              <img src={`${user.avatar}?s=50`} alt={user.name} className="size-10 rounded-full" />
              <span className="flex min-w-0 flex-col">
                <span className="truncate text-sm font-medium">{user.name}</span>
                <span className="truncate text-xs text-muted-foreground">{user.email}</span>
              </span>
            </Link>
          ) : null}

          <MenuGroup label={t('pages.settings.settings.groups.service')}>
            <MenuRow label={t('modules.connections.pages.settings.menu_item')} to={RouterPage.SETTINGS_CONNECTIONS} />
            <MenuRow label={t('modules.budget.page.settings.menu_item')} to={RouterPage.SETTINGS_BUDGETS} />
            <MenuRow label={t('pages.settings.accounts.menu_item')} to={RouterPage.SETTINGS_ACCOUNTS} />
          </MenuGroup>

          <MenuGroup label={t('pages.settings.settings.groups.classification')}>
            <MenuRow label={t('modules.classifications.categories.pages.settings.menu_item')} to={RouterPage.SETTINGS_CATEGORIES} />
            <MenuRow label={t('modules.classifications.payees.pages.settings.menu_item')} to={RouterPage.SETTINGS_PAYEES} />
            <MenuRow label={t('modules.classifications.tags.pages.settings.menu_item')} to={RouterPage.SETTINGS_TAGS} />
          </MenuGroup>

          <MenuGroup label={t('pages.settings.settings.groups.data')}>
            <MenuRow label={t('pages.settings.import_csv.menu_item')} onClick={() => setImportOpen(true)} />
            <MenuRow label={t('pages.settings.export_csv.menu_item')} onClick={() => setExportOpen(true)} />
          </MenuGroup>

          <MenuGroup label={t('pages.settings.settings.groups.preferences')}>
            <div className="flex items-center justify-between gap-2 rounded-lg bg-econumo-card px-4 py-2">
              <span className="flex flex-col">
                <span className="text-[11px] text-muted-foreground">{t('pages.settings.currency.menu_item')}</span>
              </span>
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
            {getLocaleOptions().length > 1 ? <MenuRow label={t('pages.settings.language.menu_item')} onClick={() => {}} /> : null}
          </MenuGroup>
        </div>
      </div>

      <ExportCsvDialog open={exportOpen} onClose={() => setExportOpen(false)} />
      <ImportCsvDialog open={importOpen} onClose={() => setImportOpen(false)} onComplete={setImportResult} />
      <ImportResultDialog open={importResult !== null} result={importResult} onClose={() => setImportResult(null)} />
    </div>
  )
}
