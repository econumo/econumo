import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CommandEmpty, CommandItem } from '@/components/ui/command'
import { EntityIcon } from '@/components/EntityIcon'
import { PickerSheet } from '@/components/PickerSheet'
import type { Id } from '@/api/types'
import { useOnline } from '@/hooks/useOnline'
import { apiErrorMessage } from '@/lib/apiError'
import { rankByName } from '@/lib/search'
import { useUserData } from '@/features/user/queries'
import { useAddDebtRule, useDebts } from './queries'
import { addRuleBlocker, ruleOwnerLabel } from './rules'

// up to this many debts the list is read at a glance
const SEARCH_FROM = 7

export interface AddToDebtDialogProps {
  budgetId: Id
  categoryId: Id
  open: boolean
  onOpenChange: (open: boolean) => void
}

/** A budget row's category joins one of the budget's debts as another rule,
 *  its plans moving onto the debt row */
export function AddToDebtDialog({ budgetId, categoryId, open, onOpenChange }: AddToDebtDialogProps) {
  const { t } = useTranslation()
  const online = useOnline()
  const { data: user } = useUserData()
  const { data: all = [] } = useDebts()
  const addRule = useAddDebtRule()
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (open) {
      setError(null)
    }
  }, [open, categoryId])

  const debts = all.filter((d) => d.budgetId === budgetId)
  const close = () => onOpenChange(false)
  const pick = (debtId: Id) => {
    setError(null)
    addRule.mutate({ id: debtId, rule: { categoryId } }, { onSuccess: close, onError: (err) => setError(apiErrorMessage(err)) })
  }

  return (
    <PickerSheet
      open={open}
      onClose={close}
      title={t('debts.add_to.title')}
      searchLabel={t('debts.add_to.search')}
      searchable={debts.length >= SEARCH_FROM}
      focusSearch={false}
      after={() =>
        error ? (
          <p role="alert" className="mt-3 text-sm text-destructive">
            {error}
          </p>
        ) : null
      }
    >
      {(query) => (
        <>
          <CommandEmpty className="px-1 py-2 text-left text-muted-foreground">{t('common.list.search_empty')}</CommandEmpty>
          {rankByName(debts, (d) => d.name, query).map((debt) => {
            const blocker = addRuleBlocker(all, debt.id, categoryId)
            const categories = debt.rules.map((r) => ruleOwnerLabel(r, user?.id, t)).join(', ')
            return (
              <CommandItem key={debt.id} value={debt.id} disabled={!online || addRule.isPending || blocker !== null} onSelect={() => pick(debt.id)}>
                <EntityIcon name={debt.icon} className="text-base text-muted-foreground" />
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate">{debt.name}</span>
                  <span className="truncate text-xs text-muted-foreground">{t('debts.add_to.payments_in', { categories })}</span>
                  {blocker ? (
                    <span className="truncate text-xs text-muted-foreground">
                      {t(blocker === 'covered' ? 'debts.add_to.covered' : 'debts.add_to.conflict')}
                    </span>
                  ) : null}
                </span>
              </CommandItem>
            )
          })}
        </>
      )}
    </PickerSheet>
  )
}
