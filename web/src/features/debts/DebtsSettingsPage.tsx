import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EntityIcon } from '@/components/EntityIcon'
import { InfoBox } from '@/components/InfoBox'
import { useIsCompact } from '@/hooks/useIsCompact'
import { RouterPage } from '@/app/router-pages'
import { useUiStore } from '@/app/uiStore'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { useDebts } from './queries'
import { useDebtMoney, usePaymentsLeft } from './useDebtMoney'

export function DebtsSettingsPage() {
  const { t } = useTranslation()
  const isCompact = useIsCompact()
  const { data: debts = [] } = useDebts()
  const money = useDebtMoney()
  const paymentsLeft = usePaymentsLeft()
  const openDebtModal = useUiStore((s) => s.openDebtModal)
  const openDebtDetails = useUiStore((s) => s.openDebtDetails)

  // open debts first; each group keeps the server's order
  const rows = [...debts.filter((d) => d.isPaidOff === 0), ...debts.filter((d) => d.isPaidOff === 1)]
  const createLabel = t('debts.settings.create')

  return (
    <SettingsShell
      title={t('debts.settings.header')}
      backTo={RouterPage.SETTINGS}
      actions={
        isCompact ? (
          <Button type="button" size="icon" aria-label={createLabel} title={createLabel} data-testid="debt-create" onClick={() => openDebtModal({})}>
            <Plus className="size-4" />
          </Button>
        ) : (
          <Button type="button" size="sm" data-testid="debt-create" onClick={() => openDebtModal({})}>
            <Plus className="size-4" />
            {createLabel}
          </Button>
        )
      }
    >
      <InfoBox>{t('debts.settings.info')}</InfoBox>
      {rows.length === 0 ? (
        <p className="px-1 py-2 text-sm text-muted-foreground">{t('debts.settings.empty')}</p>
      ) : (
        <ul className="flex flex-col">
          {rows.map((debt) => (
            <li key={debt.id}>
              <button
                type="button"
                data-testid={`debt-${debt.id}`}
                className={`flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-left ${isCompact ? 'active:bg-accent' : 'hover:bg-accent'}`}
                onClick={() => openDebtDetails(debt.id)}
              >
                <EntityIcon name={debt.icon} className="text-xl text-muted-foreground" />
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate text-sm">{debt.name}</span>
                  {/* without its terms a debt has no count or balance yet: the row says what is missing */}
                  {debt.isSetUp === 0 ? (
                    <span className="text-xs text-econumo-link">{t('debts.terms.finish_setup')}</span>
                  ) : (
                    <span className="text-xs text-muted-foreground">
                      {debt.isPaidOff === 1 ? <Badge variant="secondary">{t('debts.list.paid_off')}</Badge> : paymentsLeft(debt.paymentsLeft)}
                    </span>
                  )}
                </span>
                {debt.isSetUp === 0 ? null : <span className="shrink-0 text-sm">{money(debt.currentBalance, debt)}</span>}
              </button>
            </li>
          ))}
        </ul>
      )}
    </SettingsShell>
  )
}
