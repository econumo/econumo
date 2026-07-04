import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { EntityIcon } from '@/components/EntityIcon'
import { FailDialog } from '@/components/FailDialog'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { moneyFormat } from '@/lib/money'
import { downloadBlob, localDateStamp } from '@/lib/download'
import { exportTransactionList } from '@/api/transaction'
import { useAccounts } from '@/features/accounts/queries'
import { useUserData } from '@/features/user/queries'

export function ExportCsvDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useTranslation()
  const { data: accounts = [] } = useAccounts()
  const { data: user } = useUserData()

  const ownedIds = useMemo(() => accounts.filter((a) => a.owner.id === user?.id).map((a) => a.id), [accounts, user])
  const ownedKey = ownedIds.join(',')
  const [selected, setSelected] = useState<string[]>([])
  const [pending, setPending] = useState(false)
  const [failed, setFailed] = useState(false)

  // Vue pre-selects only the accounts the user owns
  useEffect(() => {
    if (open) {
      setSelected(ownedKey === '' ? [] : ownedKey.split(','))
    }
  }, [open, ownedKey])

  const hasShared = accounts.some((a) => a.owner.id !== user?.id)
  const allSelected = accounts.length > 0 && selected.length === accounts.length

  const toggle = (id: string) => {
    setSelected((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))
  }

  const handleExport = async () => {
    setPending(true)
    try {
      const blob = await exportTransactionList(selected)
      downloadBlob(blob, `transactions-${localDateStamp()}.csv`)
      onClose()
    } catch {
      setFailed(true)
    } finally {
      setPending(false)
    }
  }

  return (
    <>
      <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={t('modules.export_csv.modal.export_csv_form.header')}>
        <div className="flex flex-col gap-3">
          <div className="flex items-center justify-between">
            <p className="text-sm font-medium">{t('modules.export_csv.modal.export_csv_form.accounts')}</p>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => setSelected(allSelected ? [] : accounts.map((a) => a.id))}
            >
              {allSelected
                ? t('modules.export_csv.modal.export_csv_form.deselect_all')
                : t('modules.export_csv.modal.export_csv_form.select_all')}
            </Button>
          </div>
          <ul className="flex max-h-80 flex-col gap-1 overflow-y-auto">
            {accounts.map((account) => (
              <li key={account.id} className="flex items-center gap-3 rounded-md px-1 py-1.5">
                <EntityIcon name={account.icon} className="text-base text-muted-foreground" />
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate text-sm">{account.name}</span>
                  <span className="text-xs text-muted-foreground">
                    {moneyFormat(account.balance, account.currency)}
                    {hasShared ? <em className="ml-2 not-italic text-muted-foreground/80">{account.owner.name}</em> : null}
                  </span>
                </span>
                <Checkbox
                  aria-label={`export ${account.name}`}
                  checked={selected.includes(account.id)}
                  onCheckedChange={() => toggle(account.id)}
                />
              </li>
            ))}
          </ul>
          <div className="grid grid-cols-2 gap-3">
            <Button type="button" variant="secondary" onClick={onClose}>
              {t('elements.button.cancel.label')}
            </Button>
            <Button type="button" disabled={selected.length === 0 || pending} onClick={() => void handleExport()}>
              {t('elements.button.export.label')}
            </Button>
          </div>
        </div>
      </ResponsiveDialog>

      <FailDialog
        open={failed}
        onClose={() => setFailed(false)}
        title={t('pages.settings.export_csv.menu_item')}
        description={t('pages.settings.export_csv.error')}
      />
    </>
  )
}
