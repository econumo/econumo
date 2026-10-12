import { useTranslation } from 'react-i18next'
import { ChevronRight } from 'lucide-react'
import { Link } from 'react-router'
import type { ImportSourceDto } from '@/api/dto/imports'
import { RouterPage } from '@/app/router-pages'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { AppleWalletConnect, AppleWalletDisconnect, AppleWalletSetup } from './AppleWalletSetup'
import { ImportCards } from './ImportCards'
import { useImportQueue, useImportSources } from './queries'
import { relativeTime } from './relativeTime'
import { STEPS, useWalletSetup, type WalletSetup } from './walletSetup'

function WalletStatus({ source, setup }: { source: ImportSourceDto; setup: WalletSetup }) {
  const { t, i18n } = useTranslation()
  const { data: queue } = useImportQueue()
  const waiting =
    (queue?.queued ?? []).filter((r) => r.sourceId === source.id).length + (queue?.failed ?? []).filter((e) => e.sourceId === source.id).length
  // lastSeenAt is a fixed-width UTC datetime, so the string max is the latest
  const lastTap = source.cards.reduce((max, c) => (c.lastSeenAt > max ? c.lastSeenAt : max), '')
  // Until the first tap arrives nothing is connected in any way that matters
  // to the user, so the status names the step they are on instead.
  const settingUp = !lastTap && setup.current !== null
  const showInbox = lastTap !== '' || waiting > 0
  return (
    <div className="flex flex-col rounded-lg bg-econumo-card text-sm">
      <div className="flex items-center gap-3 px-4 py-3.5">
        <span className={`size-2 shrink-0 rounded-full ${settingUp ? 'bg-amber-500' : 'bg-income'}`} aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <div>{t(settingUp ? 'imports.apple_wallet.status.setting_up' : 'imports.apple_wallet.connected')}</div>
          <div className="text-xs text-muted-foreground">
            {settingUp && setup.current
              ? t('imports.apple_wallet.status.step', {
                  n: STEPS.indexOf(setup.current) + 1,
                  total: STEPS.length,
                  title: t(`imports.apple_wallet.steps.${setup.current}.title`),
                })
              : lastTap
                ? t('imports.apple_wallet.status.last_tap', { when: relativeTime(lastTap, i18n.language) })
                : t('imports.apple_wallet.status.no_taps')}
          </div>
        </div>
      </div>
      {showInbox ? (
        <Link
          to={RouterPage.SETTINGS_APPLE_WALLET_INBOX}
          className="flex items-center justify-between gap-2 rounded-b-lg border-t border-background px-4 py-3 hover:bg-econumo-hover"
        >
          <span>{t('inbox.title')}</span>
          <span className="flex items-center gap-2 text-muted-foreground">
            {waiting > 0 ? waiting : null}
            <ChevronRight className="size-4" />
          </span>
        </Link>
      ) : null}
    </div>
  )
}

function ConnectedWallet({ wallet }: { wallet: ImportSourceDto }) {
  const { t } = useTranslation()
  const setup = useWalletSetup(wallet)
  const hasCards = wallet.cards.length > 0
  return (
    <>
      <WalletStatus source={wallet} setup={setup} />
      {/* until the first tap arrives setup is the only thing to do, and the card list would be empty */}
      {hasCards ? <ImportCards source={wallet} footnote={t('imports.apple_wallet.same_named_cards')} /> : null}
      <AppleWalletSetup setup={setup} collapsible={hasCards} />
      <AppleWalletDisconnect source={wallet} />
    </>
  )
}

export function AppleWalletPage() {
  const { t } = useTranslation()
  const { data: sources = [] } = useImportSources()
  const wallet = sources.find((s) => s.provider === 'apple-wallet') ?? null

  return (
    <SettingsShell title={t('imports.apple_wallet.header')} backTo={RouterPage.SETTINGS}>
      <div className="mx-auto flex w-full max-w-xl flex-col gap-2">
        {/* keyed by source: the hand ticks are per source and a reconnect starts over */}
        {wallet ? <ConnectedWallet key={wallet.id} wallet={wallet} /> : <AppleWalletConnect />}
      </div>
    </SettingsShell>
  )
}
