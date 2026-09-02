import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { ImportCardDto, ImportSourceDto } from '@/api/dto/imports'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { Button } from '@/components/ui/button'
import { useAccounts } from '@/features/accounts/queries'
import { useUserData } from '@/features/user/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { dayKey, formatDayHeading } from '@/lib/datetime'
import { pluralPick } from '@/lib/plural'
import { useIgnoreImportAccount, useLinkImportAccount, useUnlinkImportAccount } from './queries'

export function ImportCards({ source }: { source: ImportSourceDto }) {
  const { t, i18n } = useTranslation()
  const { data: accounts = [] } = useAccounts()
  const { data: user } = useUserData()
  const link = useLinkImportAccount()
  const ignore = useIgnoreImportAccount()
  const unlink = useUnlinkImportAccount()
  const [mapTarget, setMapTarget] = useState<ImportCardDto | null>(null)
  const [unlinkTarget, setUnlinkTarget] = useState<ImportCardDto | null>(null)
  const [accountId, setAccountId] = useState('')
  // link-account requires ownership (the backend rejects shared accounts), so
  // the picker lists only my own accounts. Within that, accounts already in
  // the card's currency come first (Array#sort is stable, so ties keep their
  // original account-list order).
  const ownAccounts = accounts.filter((a) => a.owner.id === user?.id)
  const pickerAccounts = mapTarget
    ? [...ownAccounts].sort((a, b) => Number(b.currency.code === mapTarget.externalCurrency) - Number(a.currency.code === mapTarget.externalCurrency))
    : ownAccounts
  const accountName = (id: string) => accounts.find((a) => a.id === id)?.name ?? ''

  const submitMap = () => {
    if (!mapTarget || !accountId) {
      return
    }
    const card = mapTarget
    link.mutate(
      { sourceId: source.id, externalAccountId: card.externalAccountId, accountId },
      {
        onSuccess: (result) => {
          setMapTarget(null)
          setAccountId('')
          if (result.run) {
            toast.success(t('imports.apple_wallet.cards.mapped_toast', {
              imported: result.run.importedCount, matched: result.run.matchedCount, skipped: result.run.skippedCount,
            }))
          }
        },
        onError: (err) => toast.error(apiErrorMessage(err)),
      },
    )
  }

  const stateLabel = (card: ImportCardDto) =>
    card.state === 'mapped'
      ? `${t('imports.apple_wallet.cards.state.mapped')} · ${accountName(card.accountId)}`
      : card.state === 'ignored'
        ? t('imports.apple_wallet.cards.state.ignored')
        : t('imports.apple_wallet.cards.state.unmapped', { count: card.queuedCount })

  return (
    <div className="flex flex-col gap-2">
      <p className="px-1 pt-2 text-xs uppercase text-muted-foreground">{t('imports.apple_wallet.cards.header')}</p>
      {source.cards.length === 0 ? (
        <p className="rounded-lg bg-econumo-card px-4 py-3.5 text-sm text-muted-foreground">{t('imports.apple_wallet.cards.empty')}</p>
      ) : (
        source.cards.map((card) => (
          <div key={card.externalAccountId} className="flex flex-col gap-2 rounded-lg bg-econumo-card px-4 py-3.5 text-sm">
            <div className="flex items-center justify-between gap-2">
              <div className="min-w-0">
                <div className="truncate font-medium">{card.externalName}</div>
                <div className="text-xs text-muted-foreground">{stateLabel(card)}</div>
              </div>
              <div className="shrink-0 text-right text-xs text-muted-foreground">
                <div>{pluralPick(t('imports.apple_wallet.cards.taps'), card.tapCount, i18n.language)}</div>
                {card.lastSeenAt ? <div>{t('imports.apple_wallet.cards.last_seen', { date: formatDayHeading(dayKey(card.lastSeenAt), i18n.language) })}</div> : null}
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              {card.state === 'unmapped' ? (
                <>
                  <Button type="button" size="sm" onClick={() => setMapTarget(card)}>{t('imports.apple_wallet.cards.map')}</Button>
                  <Button type="button" size="sm" variant="secondary" disabled={ignore.isPending} onClick={() => ignore.mutate({ sourceId: source.id, externalAccountId: card.externalAccountId })}>
                    {t('imports.apple_wallet.cards.ignore')}
                  </Button>
                </>
              ) : card.state === 'ignored' ? (
                <Button type="button" size="sm" variant="secondary" onClick={() => setMapTarget(card)}>{t('imports.apple_wallet.cards.map_instead')}</Button>
              ) : (
                <Button type="button" size="sm" variant="secondary" onClick={() => setUnlinkTarget(card)}>{t('imports.apple_wallet.cards.unlink')}</Button>
              )}
            </div>
          </div>
        ))
      )}

      <ResponsiveDialog
        open={mapTarget !== null}
        onOpenChange={(o) => !o && setMapTarget(null)}
        title={t('imports.apple_wallet.cards.map_modal.header', { card: mapTarget?.externalName ?? '' })}
      >
        <div className="flex flex-col gap-3">
          <label className="text-xs uppercase text-muted-foreground" htmlFor="import-map-account">{t('imports.apple_wallet.cards.map_modal.account')}</label>
          <select id="import-map-account" className="h-11 w-full rounded-md border bg-transparent px-2 text-sm" value={accountId} onChange={(e) => setAccountId(e.target.value)}>
            <option value="" />
            {pickerAccounts.map((a) => (
              <option key={a.id} value={a.id}>{a.name}</option>
            ))}
          </select>
          <div className={dialogActionsClass}>
            <Button type="button" variant="secondary" onClick={() => setMapTarget(null)}>{t('common.button.cancel.label')}</Button>
            <Button type="button" disabled={!accountId || link.isPending} onClick={submitMap}>{t('imports.apple_wallet.cards.map_modal.submit')}</Button>
          </div>
        </div>
      </ResponsiveDialog>

      <ConfirmDialog
        open={unlinkTarget !== null}
        onClose={() => setUnlinkTarget(null)}
        onConfirm={() => {
          const card = unlinkTarget
          setUnlinkTarget(null)
          if (card) {
            unlink.mutate({ sourceId: source.id, externalAccountId: card.externalAccountId })
          }
        }}
        title={t('imports.apple_wallet.cards.unlink_modal.title', { card: unlinkTarget?.externalName ?? '' })}
        question={t('imports.apple_wallet.cards.unlink_modal.question')}
        confirmLabel={t('imports.apple_wallet.cards.unlink')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </div>
  )
}
