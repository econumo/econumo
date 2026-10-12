import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowRight, MoreVertical } from 'lucide-react'
import { toast } from 'sonner'
import type { ImportCardDto, ImportSourceDto } from '@/api/dto/imports'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EntityIcon } from '@/components/EntityIcon'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { useAccounts } from '@/features/accounts/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { pluralPick } from '@/lib/plural'
import { MapAccountDialog } from './MapAccountDialog'
import { useIgnoreImportAccount, useUnlinkImportAccount } from './queries'
import { relativeTime } from './relativeTime'

function CardMenu({ label, children }: { label: string; children: ReactNode }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="ghost" size="icon" className="shrink-0" aria-label={label}>
          <MoreVertical className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">{children}</DropdownMenuContent>
    </DropdownMenu>
  )
}

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

  const ignoreCard = (card: ImportCardDto) =>
    ignore.mutate(
      { sourceId: source.id, externalAccountId: card.externalAccountId, externalName: card.externalName },
      { onError: (err) => toast.error(apiErrorMessage(err)) },
    )

  const row = (card: ImportCardDto, main: ReactNode, sub: string, actions: ReactNode) => (
    <li key={card.externalAccountId} className="flex items-center gap-2 rounded-lg bg-econumo-card py-2.5 pl-4 pr-2 text-sm">
      <div className="min-w-0 flex-1">
        {main}
        <div className="truncate text-xs text-muted-foreground">{sub}</div>
      </div>
      {actions}
    </li>
  )

  const settledRow = (card: ImportCardDto) => {
    const menuLabel = t(`${ns}.actions`, { card: card.externalName })
    if (card.state === 'ignored') {
      return row(
        card,
        <div className="truncate text-muted-foreground">{card.externalName}</div>,
        details(t(`${ns}.state.ignored`), taps(card), when(card)),
        <CardMenu label={menuLabel}>
          <DropdownMenuItem onSelect={() => setMapTarget(card)}>{t(`${ns}.map`)}</DropdownMenuItem>
        </CardMenu>,
      )
    }
    const account = accounts.find((a) => a.id === card.accountId)
    return row(
      card,
      <div className="flex min-w-0 items-center gap-1.5">
        <span className="truncate">{card.externalName}</span>
        <ArrowRight className="size-3.5 shrink-0 text-muted-foreground" aria-label={t(`${ns}.mapped_to`)} />
        <EntityIcon name={account?.icon} className="shrink-0 text-base text-muted-foreground" />
        <span className="truncate">{account?.name ?? ''}</span>
      </div>,
      details(taps(card), when(card)),
      <CardMenu label={menuLabel}>
        <DropdownMenuItem onSelect={() => setUnlinkTarget(card)}>{t(`${ns}.unlink`)}</DropdownMenuItem>
      </CardMenu>,
    )
  }

  return (
    <div className="flex flex-col gap-2">
      {pending.length > 0 ? (
        <section className="flex flex-col gap-2">
          <h2 className="px-1 pt-2 text-xs uppercase text-muted-foreground">{t(`${ns}.needs_mapping`)}</h2>
          <ul className="flex flex-col gap-2">
            {pending.map((card) =>
              row(
                card,
                <div className="truncate">{card.externalName}</div>,
                details(pluralPick(t(`${ns}.waiting`), card.queuedCount, i18n.language), when(card)),
                <>
                  <Button type="button" size="sm" aria-label={`${t(`${ns}.map`)} ${card.externalName}`} onClick={() => setMapTarget(card)}>
                    {t(`${ns}.map_modal.submit`)}
                  </Button>
                  <CardMenu label={t(`${ns}.actions`, { card: card.externalName })}>
                    <DropdownMenuItem disabled={ignore.isPending} onSelect={() => ignoreCard(card)}>{t(`${ns}.ignore`)}</DropdownMenuItem>
                  </CardMenu>
                </>,
              ))}
          </ul>
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
