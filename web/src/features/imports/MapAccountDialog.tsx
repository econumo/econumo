import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { ImportCardDto, ImportProvider } from '@/api/dto/imports'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { Button } from '@/components/ui/button'
import { useAccounts } from '@/features/accounts/queries'
import { canWriteToAccount } from '@/features/connections/shared'
import { useUserData } from '@/features/user/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { EntitySelect } from '@/features/transactions/EntitySelect'
import { useLinkImportAccount } from './queries'

export function MapAccountDialog({ sourceId, provider, card, onClose }: { sourceId: string; provider: ImportProvider | undefined; card: ImportCardDto | null; onClose: () => void }) {
  const { t } = useTranslation()
  const ns = provider === 'simplefin' ? 'imports.simplefin.accounts' : 'imports.apple_wallet.cards'
  const { data: accounts = [] } = useAccounts()
  const { data: user } = useUserData()
  const link = useLinkImportAccount()
  // a remap starts from the card's current account; null = nothing picked yet
  const [picked, setPicked] = useState<string | null>(null)
  const current = card?.state === 'mapped' ? card.accountId : ''
  const accountId = picked ?? current
  // link-account takes any account I can add transactions to (mine, or shared
  // with me as admin/user), so the picker lists exactly those. Within that,
  // accounts already in the card's currency come first (Array#sort is stable,
  // so ties keep their original account-list order).
  const writableAccounts = accounts.filter((a) => canWriteToAccount(a, user?.id))
  const pickerAccounts = card
    ? [...writableAccounts].sort((a, b) => Number(b.currency.code === card.externalCurrency) - Number(a.currency.code === card.externalCurrency))
    : writableAccounts

  const close = () => {
    setPicked(null)
    onClose()
  }

  const submit = () => {
    if (!card || !accountId || accountId === current) {
      return
    }
    link.mutate(
      { sourceId, externalAccountId: card.externalAccountId, accountId, externalName: card.externalName },
      {
        onSuccess: (result) => {
          close()
          const run = result.run
          // a remap of a card with nothing queued converts nothing worth a toast
          if (run && run.importedCount + run.matchedCount + run.skippedCount > 0) {
            toast.success(t(`${ns}.mapped_toast`, {
              imported: run.importedCount, matched: run.matchedCount, skipped: run.skippedCount,
            }))
          }
        },
        onError: (err) => toast.error(apiErrorMessage(err)),
      },
    )
  }

  return (
    <ResponsiveDialog
      open={card !== null}
      onOpenChange={(o) => !o && close()}
      title={t(`${ns}.map_modal.header`, { card: card?.externalName ?? '' })}
    >
      <div className="flex flex-col gap-3">
        <label className="text-xs uppercase text-muted-foreground" htmlFor="import-map-account">{t(`${ns}.map_modal.account`)}</label>
        {/* the app's account picker, as on a transaction: searchable, a sheet on a phone */}
        <EntitySelect
          id="import-map-account"
          aria-label={t(`${ns}.map_modal.account`)}
          value={accountId || null}
          onChange={(id) => setPicked(id ?? '')}
          options={pickerAccounts.map((a) => ({ value: a.id, label: a.name, icon: a.icon }))}
        />
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={close}>{t('common.button.cancel.label')}</Button>
          <Button type="button" disabled={!accountId || accountId === current || link.isPending} onClick={submit}>{t(`${ns}.map_modal.submit`)}</Button>
        </div>
      </div>
    </ResponsiveDialog>
  )
}
