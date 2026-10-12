import { useTranslation } from 'react-i18next'
import { ChevronRight } from 'lucide-react'
import { Link } from 'react-router'
import type { ImportSourceDto } from '@/api/dto/imports'
import { RouterPage } from '@/app/router-pages'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { AppleWalletDisconnect, AppleWalletSetup } from './AppleWalletSetup'
import { ImportCards } from './ImportCards'
import { useImportQueue, useImportSources } from './queries'
import { relativeTime } from './relativeTime'

function WalletStatus({ source }: { source: ImportSourceDto }) {
  const { t, i18n } = useTranslation()
  const { data: queue } = useImportQueue()
  const waiting =
    (queue?.queued ?? []).filter((r) => r.sourceId === source.id).length + (queue?.failed ?? []).filter((e) => e.sourceId === source.id).length
  // lastSeenAt is a fixed-width UTC datetime, so the string max is the latest
  const lastTap = source.cards.reduce((max, c) => (c.lastSeenAt > max ? c.lastSeenAt : max), '')
  return (
    <div className="flex flex-col rounded-lg bg-econumo-card text-sm">
      <div className="flex items-center gap-3 px-4 py-3.5">
        <span className="size-2 shrink-0 rounded-full bg-income" aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <div>{t('imports.apple_wallet.connected')}</div>
          <div className="text-xs text-muted-foreground">
            {lastTap ? t('imports.apple_wallet.status.last_tap', { when: relativeTime(lastTap, i18n.language) }) : t('imports.apple_wallet.status.no_taps')}
          </div>
        </div>
      </div>
      {waiting > 0 ? (
        <Link to={RouterPage.INBOX} className="flex items-center justify-between gap-2 rounded-b-lg border-t border-background px-4 py-3 hover:bg-econumo-hover">
          <span>{t('imports.apple_wallet.status.review')}</span>
          <span className="flex items-center gap-2 text-muted-foreground">
            {waiting}
            <ChevronRight className="size-4" />
          </span>
        </Link>
      ) : null}
    </div>
  )
}

export function AppleWalletPage() {
  const { t } = useTranslation()
  const { data: sources = [] } = useImportSources()
  const wallet = sources.find((s) => s.provider === 'apple-wallet') ?? null

  return (
    <SettingsShell title={t('imports.apple_wallet.header')} backTo={RouterPage.SETTINGS}>
      <div className="mx-auto flex w-full max-w-xl flex-col gap-2">
        {wallet ? (
          <>
            <WalletStatus source={wallet} />
            <ImportCards source={wallet} footnote={t('imports.apple_wallet.same_named_cards')} />
            <AppleWalletSetup key={wallet.id} source={wallet} />
            <AppleWalletDisconnect source={wallet} />
          </>
        ) : (
          <AppleWalletSetup key="none" source={null} />
        )}
      </div>
    </SettingsShell>
  )
}
