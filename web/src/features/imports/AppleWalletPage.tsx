import { useTranslation } from 'react-i18next'
import { ChevronRight } from 'lucide-react'
import { Link } from 'react-router'
import { RouterPage } from '@/app/router-pages'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { AppleWalletSetup } from './AppleWalletSetup'
import { ImportCards } from './ImportCards'
import { useImportSources } from './queries'

export function AppleWalletPage() {
  const { t } = useTranslation()
  const { data: sources = [] } = useImportSources()
  const wallet = sources.find((s) => s.provider === 'apple-wallet') ?? null

  return (
    <SettingsShell title={t('imports.apple_wallet.header')} backTo={RouterPage.SETTINGS}>
      <div className="mx-auto flex w-full max-w-xl flex-col gap-2">
        <AppleWalletSetup key={wallet?.id ?? 'none'} source={wallet} />
        {wallet ? <ImportCards source={wallet} /> : null}
        {wallet ? (
          // the banner (queued-only) is the only other door to /imports/queue, so a
          // queue holding just skipped/needs-attention rows would otherwise be unreachable
          <Link
            to={RouterPage.IMPORT_QUEUE}
            className="flex w-full items-center justify-between gap-2 rounded-lg bg-econumo-card px-4 py-3.5 text-left text-sm hover:bg-econumo-hover"
          >
            <span>{t('imports.queue.header')}</span>
            <ChevronRight className="size-4 text-muted-foreground" />
          </Link>
        ) : null}
      </div>
    </SettingsShell>
  )
}
