import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowRight, CreditCard, EyeOff } from 'lucide-react'
import { toast } from 'sonner'
import type { ImportCardDto, ImportSourceDto } from '@/api/dto/imports'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EntityIcon } from '@/components/EntityIcon'
import { Button } from '@/components/ui/button'
import { useAccounts } from '@/features/accounts/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { pluralPick } from '@/lib/plural'
import { MapAccountDialog } from './MapAccountDialog'
import { useIgnoreImportAccount, useUnlinkImportAccount } from './queries'
import { relativeTime } from './relativeTime'

// Wraps the buttons under the text when the row is too narrow for both.
function CardRow({ tile, title, sub, actions, attention = false }: { tile: ReactNode; title: ReactNode; sub: string; actions: ReactNode; attention?: boolean }) {
  return (
    <li className={`flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg bg-econumo-card px-4 py-3 text-sm ${attention ? 'ring-1 ring-econumo-purple/40' : ''}`}>
      <div className="flex min-w-0 flex-1 basis-56 items-center gap-3">
        {tile}
        <div className="min-w-0 flex-1">
          {title}
          <div className="truncate text-xs text-muted-foreground">{sub}</div>
        </div>
      </div>
      <div className="ml-auto flex shrink-0 items-center gap-1">{actions}</div>
    </li>
  )
}

const tileClass = 'flex size-10 shrink-0 items-center justify-center rounded-lg'

export function ImportCards({ source, cards = source.cards, variant = 'card', footnote }: { source: ImportSourceDto; cards?: ImportCardDto[]; variant?: 'card' | 'account'; footnote?: ReactNode }) {
  const { t, i18n } = useTranslation()
  const ns = variant === 'account' ? 'imports.simplefin.accounts' : 'imports.apple_wallet.cards'
  const { data: accounts = [] } = useAccounts()
  const ignore = useIgnoreImportAccount()
  const unlink = useUnlinkImportAccount()
  const [mapTarget, setMapTarget] = useState<ImportCardDto | null>(null)
  const [unlinkTarget, setUnlinkTarget] = useState<ImportCardDto | null>(null)

  const pending = cards.filter((c) => c.state === 'unmapped')
  // mapped before ignored; Array#sort is stable, so each keeps the server order
  const settled = cards.filter((c) => c.state !== 'unmapped').sort((a, b) => Number(a.state === 'ignored') - Number(b.state === 'ignored'))
  const when = (card: ImportCardDto) => (card.lastSeenAt ? relativeTime(card.lastSeenAt, i18n.language) : '')
  const details = (...parts: string[]) => parts.filter(Boolean).join(' · ')
  const taps = (card: ImportCardDto) => pluralPick(t(`${ns}.taps`), card.tapCount, i18n.language)
  const action = (label: string, card: ImportCardDto) => `${label} ${card.externalName}`

  const ignoreCard = (card: ImportCardDto) =>
    ignore.mutate(
      { sourceId: source.id, externalAccountId: card.externalAccountId, externalName: card.externalName },
      { onError: (err) => toast.error(apiErrorMessage(err)) },
    )

  const pendingRow = (card: ImportCardDto) => (
    <CardRow
      key={card.externalAccountId}
      attention
      tile={<span className={`${tileClass} bg-econumo-purple/10 text-econumo-purple`}><CreditCard className="size-5" /></span>}
      title={<div className="truncate">{card.externalName}</div>}
      sub={details(pluralPick(t(`${ns}.waiting`), card.queuedCount, i18n.language), when(card))}
      actions={
        <>
          <Button type="button" size="sm" aria-label={action(t(`${ns}.map`), card)} onClick={() => setMapTarget(card)}>
            {t(`${ns}.map_modal.submit`)}
          </Button>
          <Button type="button" size="sm" variant="ghost" aria-label={action(t(`${ns}.ignore`), card)} disabled={ignore.isPending} onClick={() => ignoreCard(card)}>
            {t(`${ns}.ignore`)}
          </Button>
        </>
      }
    />
  )

  const settledRow = (card: ImportCardDto) => {
    if (card.state === 'ignored') {
      return (
        <CardRow
          key={card.externalAccountId}
          tile={<span className={`${tileClass} bg-background text-muted-foreground`}><EyeOff className="size-5" /></span>}
          title={<div className="truncate text-muted-foreground">{card.externalName}</div>}
          sub={details(t(`${ns}.state.ignored`), taps(card), when(card))}
          actions={
            <Button type="button" size="sm" variant="secondary" aria-label={action(t(`${ns}.map`), card)} onClick={() => setMapTarget(card)}>
              {t(`${ns}.map_modal.submit`)}
            </Button>
          }
        />
      )
    }
    const account = accounts.find((a) => a.id === card.accountId)
    return (
      <CardRow
        key={card.externalAccountId}
        tile={<span className={`${tileClass} bg-background`}><EntityIcon name={account?.icon} className="text-lg text-[#666666]" /></span>}
        title={
          <div className="flex min-w-0 items-center gap-1.5">
            <span className="truncate">{card.externalName}</span>
            <ArrowRight className="size-3.5 shrink-0 text-muted-foreground" aria-label={t(`${ns}.mapped_to`)} />
            <span className="truncate">{account?.name ?? ''}</span>
          </div>
        }
        sub={details(taps(card), when(card))}
        actions={
          <>
            <Button type="button" size="sm" variant="secondary" aria-label={action(t(`${ns}.remap`), card)} onClick={() => setMapTarget(card)}>
              {t(`${ns}.remap`)}
            </Button>
            <Button type="button" size="sm" variant="ghost" aria-label={action(t(`${ns}.unlink`), card)} onClick={() => setUnlinkTarget(card)}>
              {t(`${ns}.unlink`)}
            </Button>
          </>
        }
      />
    )
  }

  return (
    <div className="flex flex-col gap-2">
      {pending.length > 0 ? (
        <section className="flex flex-col gap-2">
          <h2 className="px-1 pt-2 text-xs uppercase text-muted-foreground">{t(`${ns}.needs_mapping`)}</h2>
          <ul className="flex flex-col gap-2">{pending.map(pendingRow)}</ul>
        </section>
      ) : null}

      {settled.length > 0 || pending.length === 0 ? (
        <section className="flex flex-col gap-2">
          <h2 className="px-1 pt-2 text-xs uppercase text-muted-foreground">{t(`${ns}.header`)}</h2>
          {settled.length === 0 ? (
            <p className="rounded-lg bg-econumo-card px-4 py-3.5 text-sm text-muted-foreground">{t(`${ns}.empty`)}</p>
          ) : (
            <ul className="flex flex-col gap-2">{settled.map(settledRow)}</ul>
          )}
        </section>
      ) : null}

      {footnote ? <p className="px-1 text-xs text-muted-foreground">{footnote}</p> : null}

      <MapAccountDialog sourceId={source.id} provider={source.provider} card={mapTarget} onClose={() => setMapTarget(null)} />

      <ConfirmDialog
        open={unlinkTarget !== null}
        onClose={() => setUnlinkTarget(null)}
        onConfirm={() => {
          const card = unlinkTarget
          setUnlinkTarget(null)
          if (card) {
            unlink.mutate(
              { sourceId: source.id, externalAccountId: card.externalAccountId },
              { onError: (err) => toast.error(apiErrorMessage(err)) },
            )
          }
        }}
        title={t(`${ns}.unlink_modal.title`, { card: unlinkTarget?.externalName ?? '' })}
        question={t(`${ns}.unlink_modal.question`)}
        confirmLabel={t(`${ns}.unlink`)}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </div>
  )
}
