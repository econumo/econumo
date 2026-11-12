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
import type { DebtDto } from '@/api/dto/debt'
import { useDebts, usePlannableBudgets } from './queries'
import { useDebtMoney, usePaymentsLeft } from './useDebtMoney'

interface BudgetGroup {
  budgetId: string
  budgetName: string
  debts: DebtDto[]
}

/** the debts under their budgets, in the server's order; open debts first in each */
function byBudget(debts: DebtDto[]): BudgetGroup[] {
  const groups: BudgetGroup[] = []
  for (const debt of debts) {
    let group = groups.find((g) => g.budgetId === debt.budgetId)
    if (!group) {
      group = { budgetId: debt.budgetId, budgetName: debt.budgetName, debts: [] }
      groups.push(group)
    }
    group.debts.push(debt)
  }
  return groups.map((g) => ({ ...g, debts: [...g.debts.filter((d) => d.isPaidOff === 0), ...g.debts.filter((d) => d.isPaidOff === 1)] }))
}

export function DebtsSettingsPage() {
  const { t } = useTranslation()
  const isCompact = useIsCompact()
  const { data: debts = [] } = useDebts()
  const money = useDebtMoney()
  const paymentsLeft = usePaymentsLeft()
  const openDebtModal = useUiStore((s) => s.openDebtModal)
  const openDebtDetails = useUiStore((s) => s.openDebtDetails)
  const plannable = usePlannableBudgets()

  const groups = byBudget(debts)
  const createLabel = t('debts.settings.create')
  // a debt belongs to a budget the user plans in; still loading counts as none yet, silently
  const noBudget = plannable !== undefined && plannable.length === 0
  const canCreate = (plannable?.length ?? 0) > 0

  return (
    <SettingsShell
      title={t('debts.settings.header')}
      backTo={RouterPage.SETTINGS}
      actions={
        isCompact ? (
          <Button type="button" size="icon" aria-label={createLabel} title={createLabel} data-testid="debt-create" disabled={!canCreate} onClick={() => openDebtModal({})}>
            <Plus className="size-4" />
          </Button>
        ) : (
          <Button type="button" size="sm" data-testid="debt-create" disabled={!canCreate} onClick={() => openDebtModal({})}>
            <Plus className="size-4" />
            {createLabel}
          </Button>
        )
      }
    >
      <InfoBox>{t('debts.settings.info')}</InfoBox>
      {noBudget ? <p className="px-1 text-sm text-muted-foreground">{t('debts.settings.no_budget')}</p> : null}
      {groups.length === 0 ? (
        <p className="px-1 py-2 text-sm text-muted-foreground">{t('debts.settings.empty')}</p>
      ) : (
        groups.map((group) => (
          <section key={group.budgetId} aria-labelledby={`debts-budget-${group.budgetId}`} className="flex flex-col gap-1">
            <h3 id={`debts-budget-${group.budgetId}`} className="px-3 text-xs uppercase tracking-wide text-muted-foreground">
              {group.budgetName}
            </h3>
            <ul className="flex flex-col">
              {group.debts.map((debt) => (
                <li key={debt.id}>
                  <button
                    type="button"
                    data-testid={`debt-${debt.id}`}
                    className={`flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-left ${isCompact ? 'active:bg-accent' : 'hover:bg-accent'}`}
                    // a debt without terms says Finish setup to those who may finish it: the row opens the form for it
                    onClick={() => openDebtDetails(debt.id, debt.isSetUp === 0 && debt.canEdit === 1 ? 'settings' : 'overview')}
                  >
                    <EntityIcon name={debt.icon} className="text-xl text-muted-foreground" />
                    <span className="flex min-w-0 flex-1 flex-col">
                      <span className="truncate text-sm">{debt.name}</span>
                      {/* without its terms a debt has no count or balance yet: the row says what is missing */}
                      {debt.isSetUp === 0 ? (
                        debt.canEdit === 1 ? <span className="text-xs text-econumo-link">{t('debts.terms.finish_setup')}</span> : null
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
          </section>
        ))
      )}
    </SettingsShell>
  )
}
