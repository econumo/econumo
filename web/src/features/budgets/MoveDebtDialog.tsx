import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { EntityIcon } from '@/components/EntityIcon'
import { apiErrorMessage } from '@/lib/apiError'
import { useOnline } from '@/hooks/useOnline'
import type { BudgetDto, BudgetElementDto } from '@/api/dto/budget'
import { BudgetElementType } from '@/api/dto/budget'
import type { DebtDto } from '@/api/dto/debt'
import type { Id } from '@/api/types'
import { useMoveDebt } from '@/features/debts/queries'

export interface MoveDebtTarget {
  debtId: Id
  /** the element the debt is on now */
  elementId: Id
}

/** the elements the debt can move to, as move-debt checks them: a top-level
 *  expense category or an expense envelope, not archived, in the debt's
 *  currency (its element's), with no active debt */
export function moveDebtCandidates(budget: BudgetDto, target: MoveDebtTarget, debts: DebtDto[] | undefined): BudgetElementDto[] {
  const elements = budget.structure.elements
  const from = elements.find((el) => el.id === target.elementId)
  const currencyOf = (el: BudgetElementDto) => el.currencyId ?? budget.meta.currencyId
  const busy = new Set(
    (debts ?? []).filter((d) => d.budgetId === budget.meta.id && d.status === 'active').map((d) => d.elementId),
  )
  return elements
    .filter(
      (el) =>
        (el.type === BudgetElementType.CATEGORY || el.type === BudgetElementType.ENVELOPE) &&
        el.id !== target.elementId &&
        el.isArchived !== 1 &&
        (!from || currencyOf(el) === currencyOf(from)) &&
        !busy.has(el.id) &&
        !(el.debt && el.debt.isPaidOff === 0),
    )
    .sort((a, b) => a.name.localeCompare(b.name))
}

/** Picks the category or envelope a debt moves to, with its history. An
 *  envelope counts all its categories' spending as payments, so it says so. */
export function MoveDebtDialog({
  budget,
  target,
  debts,
  onClose,
}: {
  budget: BudgetDto
  target: MoveDebtTarget | null
  debts: DebtDto[] | undefined
  onClose: () => void
}) {
  const { t } = useTranslation()
  const online = useOnline()
  const move = useMoveDebt()
  const [error, setError] = useState<string | null>(null)
  if (!target) {
    return null
  }
  const candidates = moveDebtCandidates(budget, target, debts)
  const close = () => {
    setError(null)
    onClose()
  }
  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && close()} title={t('debts.move.title')} description={t('debts.move.hint')}>
      {candidates.length === 0 ? (
        <p className="text-sm text-muted-foreground" data-testid="move-debt-empty">
          {t('debts.move.empty')}
        </p>
      ) : (
        <ul className="flex max-h-80 flex-col overflow-y-auto scrollbar-slim" data-testid="move-debt-targets">
          {candidates.map((el) => (
            <li key={el.id}>
              <button
                type="button"
                disabled={!online || move.isPending}
                className="flex w-full items-center gap-3 rounded-md px-2 py-2 text-left hover:bg-econumo-hover disabled:opacity-50"
                onClick={() => {
                  setError(null)
                  move.mutate(
                    { id: target.debtId, elementId: el.id },
                    { onSuccess: close, onError: (err) => setError(apiErrorMessage(err)) },
                  )
                }}
              >
                <EntityIcon name={el.icon} className="text-lg leading-none text-muted-foreground" />
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate text-sm">{el.name}</span>
                  {el.type === BudgetElementType.ENVELOPE ? (
                    <span className="truncate text-xs text-muted-foreground">{t('debts.move.envelope_note')}</span>
                  ) : null}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
    </ResponsiveDialog>
  )
}
