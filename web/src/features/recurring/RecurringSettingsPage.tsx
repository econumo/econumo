import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { RecurringDto } from '@/api/dto/recurring'
import { EntityIcon } from '@/components/EntityIcon'
import { Button } from '@/components/ui/button'
import { RouterPage } from '@/app/router-pages'
import { useUiStore } from '@/app/uiStore'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories, usePayees } from '@/features/classifications/queries'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { moneyFormat } from '@/lib/money'
import { dayKey, isFuture } from '@/lib/datetime'
import { useRecurring } from './queries'

export function RecurringSettingsPage() {
  const { t } = useTranslation()
  const { data: recurring = [] } = useRecurring()
  const { data: accounts } = useAccounts()
  const { data: categories } = useCategories()
  const { data: payees } = usePayees()
  const openRecurringModal = useUiStore((s) => s.openRecurringModal)
  const [selected, setSelected] = useState<RecurringDto | null>(null)

  const scheduleLabel = (rt: RecurringDto) => t(`modals.recurring.schedule.${rt.schedule}`)
  const accountOf = (rt: RecurringDto) => accounts?.find((a) => a.id === rt.accountId)
  const title = (rt: RecurringDto) =>
    rt.description ||
    payees?.find((p) => p.id === rt.payeeId)?.name ||
    categories?.find((c) => c.id === rt.categoryId)?.name ||
    scheduleLabel(rt)

  return (
    <SettingsShell
      title={t('pages.settings.recurring.header')}
      backTo={RouterPage.SETTINGS}
      actions={
        <Button type="button" size="sm" data-testid="recurring-create" onClick={() => openRecurringModal({})}>
          {t('pages.settings.recurring.create')}
        </Button>
      }
    >
      {recurring.length === 0 ? (
        <p className="px-1 py-2 text-sm text-muted-foreground">{t('pages.settings.recurring.empty')}</p>
      ) : (
        recurring.map((rt) => (
          <div
            key={rt.id}
            data-testid={`recurring-${rt.id}`}
            className="flex cursor-pointer items-center gap-3 rounded-md p-2 hover:bg-accent"
            onClick={() => setSelected(rt)}
          >
            <EntityIcon
              name={rt.type === 'transfer' ? 'sync_alt' : (categories?.find((c) => c.id === rt.categoryId)?.icon ?? 'question_mark')}
              className="text-muted-foreground"
            />
            <div className="min-w-0 flex-1">
              <p className="truncate">{title(rt)}</p>
              {/* a past next-payment date means the template needs attention (post or skip) */}
              <p
                data-testid={`recurring-summary-${rt.id}`}
                className={`text-sm ${isFuture(rt.nextPaymentAt) ? 'text-muted-foreground' : 'text-destructive'}`}
              >
                <span>{scheduleLabel(rt)}</span> · <span>{dayKey(rt.nextPaymentAt)}</span>
              </p>
            </div>
            <p className="text-sm">
              {moneyFormat(rt.amount, accountOf(rt)?.currency, { useNativePrecision: false })}
            </p>
          </div>
        ))
      )}
      {/* selected -> ViewRecurringDialog wired in Task 14; keep state so the row click is testable */}
      {selected ? <span data-testid="recurring-selected" className="hidden" /> : null}
    </SettingsShell>
  )
}
