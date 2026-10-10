import { useTranslation } from 'react-i18next'
import type { BudgetMode } from './budgetStore'

const VIEWS: { mode: BudgetMode; labelKey: string }[] = [
  { mode: 'budget', labelKey: 'budgets.page.plan.toggle.budget' },
  { mode: 'plan', labelKey: 'budgets.page.plan.toggle.plan' },
]

/** Budget / Plan as plain words at the start of the month navigation row: the views
 *  decide how many months show, so they sit with the months they control. */
export function ViewSwitch({ mode, onSwitch }: { mode: BudgetMode; onSwitch: (mode: BudgetMode) => void }) {
  const { t } = useTranslation()
  return (
    <div role="tablist" aria-label="budget mode" className="mr-1 flex shrink-0 items-center gap-3 border-r pr-3">
      {VIEWS.map((v) => (
        <button
          key={v.mode}
          type="button"
          role="tab"
          aria-selected={mode === v.mode}
          className={`border-b-2 pt-0.5 pb-px text-sm ${
            mode === v.mode ? 'border-foreground font-semibold text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground'
          }`}
          onClick={() => onSwitch(v.mode)}
        >
          {/* an invisible bold copy holds the bold width, so the words never shift
              when the selection moves between them */}
          <span className="inline-grid">
            <span aria-hidden="true" className="invisible col-start-1 row-start-1 font-semibold">
              {t(v.labelKey)}
            </span>
            <span className="col-start-1 row-start-1">{t(v.labelKey)}</span>
          </span>
        </button>
      ))}
    </div>
  )
}
