import { useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { ChevronLeft } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { UserAvatar } from '@/components/UserAvatar'
import { getVersion, backendHost, getWebsiteUrl } from '@/lib/config'
import { useAvailableUpdate } from '@/hooks/useAvailableUpdate'
import { useIsCompact } from '@/hooks/useIsCompact'
import { useNavigate } from 'react-router'
import { RouterPage } from '@/app/router-pages'
import { dayKey, formatDayHeading } from '@/lib/datetime'
import { useUserData, useAccessState } from '@/features/user/queries'
import { useOpenBillingPortal } from '@/features/access/useOpenBillingPortal'
import { ExportCsvDialog } from '@/features/transactions/ExportCsvDialog'
import { ImportCsvDialog } from '@/features/transactions/ImportCsvDialog'
import { ImportResultDialog } from '@/features/transactions/ImportResultDialog'
import type { AggregatedImportResult } from '@/features/transactions/importCsv'
import { SEMVER } from '@/lib/version'

function MenuGroup({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <>
      <p className="px-1 pt-2 text-xs uppercase text-muted-foreground">{label}</p>
      <nav className="flex flex-col gap-2">{children}</nav>
    </>
  )
}

// Vue renders the hub as light-gray card rows in a narrow column
function MenuRow({
  label,
  to,
  onClick,
  trailing,
  accent,
}: {
  label: string
  to?: string
  onClick?: () => void
  trailing?: React.ReactNode
  // 'hover' tints the row on hover only; 'rest' keeps the accent visible at rest
  accent?: 'hover' | 'rest'
}) {
  const inner = (
    <span
      className={`flex w-full items-center justify-between gap-2 rounded-lg px-4 py-3.5 text-sm ${
        accent === 'rest'
          ? 'bg-primary/10 text-primary hover:bg-primary/15'
          : accent === 'hover'
            ? 'bg-econumo-card hover:bg-primary/10 hover:text-primary'
            : 'bg-econumo-card hover:bg-econumo-hover'
      }`}
    >
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
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const isCompact = useIsCompact()
  const { data: user } = useUserData()
  const [exportOpen, setExportOpen] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [importResult, setImportResult] = useState<AggregatedImportResult | null>(null)
  const version = getVersion()
  const update = useAvailableUpdate()
  const access = useAccessState()
  const portal = useOpenBillingPortal()

  return (
    <div className="flex h-full flex-col gap-3 p-4">
      {isCompact ? (
        <header className="flex items-center gap-2">
          <Button type="button" variant="ghost" size="icon" aria-label="back" onClick={() => navigate(RouterPage.HOME)}>
            <ChevronLeft className="size-5" />
          </Button>
          <h1 className="flex-1 truncate text-lg font-semibold">{t('settings.page.header')}</h1>
        </header>
      ) : (
        <h1 className="text-xl font-semibold">{t('settings.page.header_desktop')}</h1>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="flex max-w-md flex-col gap-2">
          {user ? (
            <Link
              to={RouterPage.SETTINGS_PROFILE}
              className="flex items-center gap-3 rounded-lg bg-econumo-card px-4 py-3 hover:bg-econumo-hover"
            >
              <UserAvatar avatar={user.avatar} size="md" />
              <span className="flex min-w-0 flex-col">
                <span className="truncate text-sm font-medium">{user.name}</span>
                <span className="truncate text-xs text-muted-foreground">{user.email}</span>
              </span>
            </Link>
          ) : null}

          {update ? (
            <a
              href={update.url}
              target="_blank"
              rel="noreferrer"
              className="flex items-center justify-between gap-2 rounded-lg bg-primary/10 px-4 py-3.5 text-sm font-medium text-primary hover:bg-primary/15"
            >
              <span>{t('settings.update.available', { version: update.version })}</span>
              <ChevronRight className="size-4" />
            </a>
          ) : null}

          {access.billingEnabled ? (
            <MenuGroup label={t('subscription.settings.group')}>
              <MenuRow
                accent={access.state === 'full_access' ? 'hover' : 'rest'}
                label={t('subscription.settings.portal')}
                onClick={() => {
                  if (!portal.pending) portal.open()
                }}
                trailing={
                  access.state === 'trial' ? (
                    <span className="shrink-0 text-xs text-primary/80">
                      {t('subscription.settings.status.trial', { date: formatDayHeading(dayKey(access.accessUntil), i18n.language) })}
                    </span>
                  ) : access.state === 'readonly' ? (
                    <span className="shrink-0 text-xs text-destructive">{t('subscription.settings.status.readonly')}</span>
                  ) : undefined
                }
              />
            </MenuGroup>
          ) : null}

          <MenuGroup label={t('settings.page.groups.service')}>
            <MenuRow label={t('settings.accounts.menu_item')} to={RouterPage.SETTINGS_ACCOUNTS} />
            <MenuRow label={t('connections.pages.settings.menu_item')} to={RouterPage.SETTINGS_CONNECTIONS} />
            <MenuRow label={t('budgets.page.settings.menu_item')} to={RouterPage.SETTINGS_BUDGETS} />
          </MenuGroup>

          <MenuGroup label={t('settings.page.groups.classification')}>
            <MenuRow label={t('classifications.categories.pages.settings.menu_item')} to={RouterPage.SETTINGS_CATEGORIES} />
            <MenuRow label={t('classifications.tags.pages.settings.menu_item')} to={RouterPage.SETTINGS_TAGS} />
            <MenuRow label={t('classifications.payees.pages.settings.menu_item')} to={RouterPage.SETTINGS_PAYEES} />
          </MenuGroup>

          <MenuGroup label={t('settings.page.groups.data')}>
            <MenuRow label={t('settings.import_csv.menu_item')} onClick={() => setImportOpen(true)} />
            <MenuRow label={t('settings.export_csv.menu_item')} onClick={() => setExportOpen(true)} />
          </MenuGroup>

        </div>
      </div>

      <footer className="flex items-center justify-center gap-2 py-1 text-xs text-muted-foreground/60">
        {SEMVER.test(version) ? (
          <a
            href={`${getWebsiteUrl()}/releases/${version}/`}
            target="_blank"
            rel="noreferrer"
            className="transition-colors hover:text-muted-foreground"
          >
            Econumo {version}
          </a>
        ) : (
          <span>Econumo {version}</span>
        )}
        <span aria-hidden="true">·</span>
        <a
          href={`${backendHost()}/api/doc`}
          target="_blank"
          rel="noreferrer"
          className="transition-colors hover:text-muted-foreground"
        >
          {t('settings.page.footer.api')}
        </a>
      </footer>


      <ExportCsvDialog open={exportOpen} onClose={() => setExportOpen(false)} />
      <ImportCsvDialog open={importOpen} onClose={() => setImportOpen(false)} onComplete={setImportResult} />
      <ImportResultDialog open={importResult !== null} result={importResult} onClose={() => setImportResult(null)} />
    </div>
  )
}
