import { useTranslation } from 'react-i18next'
import { ChevronRight } from 'lucide-react'
import { Link } from 'react-router'
import type { ImportSourceDto } from '@/api/dto/imports'
import { RouterPage } from '@/app/router-pages'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { AppleWalletConnect, AppleWalletDisconnect, AppleWalletSetup } from './AppleWalletSetup'
import { ImportCards } from './ImportCards'
import { useImportSources } from './queries'
import { useWalletSetup } from './walletSetup'

function ConnectedWallet({ wallet }: { wallet: ImportSourceDto }) {
  const { t } = useTranslation()
  const setup = useWalletSetup(wallet)
  const hasCards = wallet.cards.length > 0
  const hasTaps = wallet.cards.some((card) => card.tapCount > 0)
  return (
    <>
      {hasTaps ? (
        <Link
          to={RouterPage.SETTINGS_APPLE_WALLET_HISTORY}
          className="flex items-center justify-between gap-2 rounded-lg bg-econumo-card px-4 py-3 text-sm hover:bg-econumo-hover"
        >
          <span>{t('imports.runs.history_link')}</span>
          <ChevronRight className="size-4 text-muted-foreground" />
        </Link>
      ) : null}
      {/* until the first tap arrives setup is the only thing to do, and the card list would be empty */}
      {hasCards ? <ImportCards source={wallet} footnote={t('imports.apple_wallet.same_named_cards')} /> : null}
      <AppleWalletSetup setup={setup} />
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
