import type { ReactNode } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { EntityIcon } from '@/components/EntityIcon'
import { add, isZero } from '@/lib/decimal'
import { moneyFormat } from '@/lib/money'
import type { MoneyFormatOptions } from '@/lib/money'
import type { BudgetDto, BudgetSavingsElementDto } from '@/api/dto/budget'
import { BudgetElementType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { elementDisplayName, makeBudgetExchange, totalsWithSavings } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'
import type { BudgetTransactionsTarget } from './BudgetTransactionsDialog'
import type { PlanCellFigures, PlanMonthFigures, SheetTarget } from './phoneMonth'

const EMPTY_CELL = '—'
// the budget table's columns: name | Budget w-24 | Spent w-20/24 | Available w-20/24 | symbol w-6
const PLANNED_COL = 'hidden w-24 shrink-0 text-right sm:block'
const ACTUAL_COL = 'w-20 shrink-0 text-center sm:w-24'
const THIRD_COL = 'w-20 shrink-0 text-center sm:w-24'
const SYMBOL_COL = 'hidden w-6 shrink-0 text-center text-xs text-muted-foreground sm:block'

export type FlowTarget = Extract<SheetTarget, { kind: 'plan' } | { kind: 'savings' }>

const cellOpts = (currency: CurrencyDto | undefined): MoneyFormatOptions => ({
  showCurrency: false,
  useNativePrecision: false,
  maxPrecision: currency?.fractionDigits ?? 2,
})

/** The section line: open, it names the section's columns; folded, it carries the
 *  section's sums in those columns. The fold state is the one the Plan view uses. */
export function MonthSectionHeader({
  foldKey,
  label,
  headings,
  sums,
  actionsColumn,
  testId,
}: {
  foldKey: string
  label: string
  headings: [string, string, string]
  sums: [string, string, string]
  actionsColumn: boolean
  testId: string
}) {
  const { t } = useTranslation()
  const folded = useBudgetPeriodStore((s) => !!s.planFolds[foldKey])
  const toggle = useBudgetPeriodStore((s) => s.togglePlanFold)
  const Chevron = folded ? ChevronRight : ChevronDown
  const cells = folded ? sums : headings
  return (
    <div
      className={`flex items-center gap-1.5 px-3 sm:gap-2 sm:px-4 ${folded ? 'text-sm tabular-nums' : 'text-[11px] uppercase tracking-wide text-muted-foreground'}`}
      data-testid={testId}
    >
      <button
        type="button"
        aria-expanded={!folded}
        title={t(folded ? 'common.button.expand.label' : 'common.button.collapse.label')}
        onClick={() => toggle(foldKey)}
        className="flex min-w-0 flex-1 items-center gap-1.5 py-1 text-left text-sm font-semibold normal-case tracking-normal text-foreground"
      >
        <Chevron aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
        <span className="truncate">{label}</span>
      </button>
      <span className={PLANNED_COL}>{cells[0]}</span>
      <span className={ACTUAL_COL}>{cells[1]}</span>
      <span className={THIRD_COL}>{cells[2]}</span>
      <span className={SYMBOL_COL} />
      {actionsColumn ? <span data-testid="actions-spacer" className="w-8 shrink-0" /> : null}
    </div>
  )
}

function useSectionFolded(foldKey: string): boolean {
  return useBudgetPeriodStore((s) => !!s.planFolds[foldKey])
}

function FlowRow({
  testId,
  icon,
  name,
  muted = false,
  planned,
  actual,
  third,
  currency,
  actionsColumn,
  toggle,
}: {
  testId: string
  icon: string
  name: string
  muted?: boolean
  planned: ReactNode
  actual: ReactNode
  third: ReactNode
  currency: CurrencyDto | undefined
  actionsColumn: boolean
  /** an envelope's name folds and unfolds its categories */
  toggle?: { open: boolean; onToggle: () => void }
}) {
  const { t } = useTranslation()
  const Chevron = toggle?.open ? ChevronDown : ChevronRight
  const label = (
    <>
      {toggle ? <Chevron className="hidden size-3.5 shrink-0 text-muted-foreground sm:block" /> : <span className="hidden w-3.5 shrink-0 sm:block" />}
      <EntityIcon name={icon} className="text-lg text-muted-foreground" />
      <span className={`min-w-0 truncate text-[15px] ${muted ? 'text-muted-foreground' : ''}`} title={name}>
        {name}
      </span>
    </>
  )
  return (
    <div className="flex items-center gap-1.5 rounded-md px-1.5 py-2.5 hover:bg-accent/50 sm:gap-2 sm:px-2" data-testid={testId}>
      {toggle ? (
        <button
          type="button"
          aria-expanded={toggle.open}
          title={t(toggle.open ? 'common.button.collapse.label' : 'common.button.expand.label')}
          onClick={toggle.onToggle}
          className="flex min-w-0 flex-1 items-center gap-2 text-left"
        >
          {label}
        </button>
      ) : (
        <span className="flex min-w-0 flex-1 items-center gap-2">{label}</span>
      )}
      <span className={`${PLANNED_COL} text-[15px] tabular-nums`} data-testid="flow-planned">
        {planned}
      </span>
      <span className={`${ACTUAL_COL} flex justify-center text-[15px] tabular-nums text-muted-foreground`} data-testid="flow-actual">
        {actual}
      </span>
      <span className={`${THIRD_COL} text-[15px] tabular-nums`} data-testid="flow-third">
        {third}
      </span>
      <span className={SYMBOL_COL}>{currency?.symbol}</span>
      {actionsColumn ? <span className="w-8 shrink-0" /> : null}
    </div>
  )
}

interface MonthFlowsProps {
  budget: BudgetDto
  currencies: CurrencyDto[]
  /** null while get-budget-plan loads: the Income section waits for it */
  planMonth: PlanMonthFigures | null
  future: boolean
  actionsColumn: boolean
  /** the planned cell: the page decides between the inline editor, the item sheet and comments */
  renderPlanned: (target: FlowTarget, text: string) => ReactNode
  onShowTransactions?: (target: BudgetTransactionsTarget) => void
}

/** Income and Savings for one month, above the expenses table: the phone's order and
 *  figures (Planned · Received, Planned · Saved · Balance) in the table's columns. */
export function MonthFlows({ budget, currencies, planMonth, future, actionsColumn, renderPlanned, onShowTransactions }: MonthFlowsProps) {
  const { t } = useTranslation()
  const base = budget.meta.currencyId
  const currencyOf = (id: Id | null) => currencies.find((c) => c.id === (id ?? base))
  const fmt = (amount: string, currencyId: Id | null = base) => {
    const c = currencyOf(currencyId)
    return moneyFormat(amount, c, cellOpts(c))
  }
  const exchangeFn = makeBudgetExchange(budget, currencies)
  const unfolded = useBudgetPeriodStore((s) => s.unfoldedElements)
  const toggleElement = useBudgetPeriodStore((s) => s.toggleElement)
  const incomeFolded = useSectionFolded('income')
  const savingsFolded = useSectionFolded('savings')

  const actualCell = (target: BudgetTransactionsTarget | null, amount: string, currencyId: Id | null) => {
    if (future) {
      return EMPTY_CELL
    }
    const text = fmt(amount, currencyId)
    if (!target || !onShowTransactions) {
      return text
    }
    return (
      <button
        type="button"
        title={t('budgets.page.budget.structure.element.action.show_transactions')}
        aria-label={`transactions ${target.name}`}
        className="underline-offset-2 hover:text-foreground hover:underline"
        onClick={() => onShowTransactions(target)}
      >
        {text}
      </button>
    )
  }

  const incomeRow = (row: PlanCellFigures) => {
    const el = row.element
    const name = elementDisplayName(el.id, el.name, t)
    // income Uncategorized gathers income booked in expense categories too: no list names it
    const listTarget = el.id === UNCATEGORIZED_ID ? null : { id: el.id, type: el.type, name, icon: el.icon, currencyId: el.currencyId }
    const expandable = el.children.length > 0
    const open = expandable && !!unfolded[el.id]
    return (
      <div key={`${el.id}:${el.type}`}>
        <FlowRow
          testId={`month-income-row-${el.id}`}
          icon={el.icon}
          name={name}
          muted={el.isArchived === 1}
          planned={el.id === UNCATEGORIZED_ID ? EMPTY_CELL : renderPlanned({ kind: 'plan', cell: row }, fmt(row.planned, el.currencyId))}
          actual={actualCell(listTarget, row.actual, el.currencyId)}
          third={null}
          currency={currencyOf(el.currencyId)}
          actionsColumn={actionsColumn}
          toggle={expandable ? { open, onToggle: () => toggleElement(el.id) } : undefined}
        />
        {open ? (
          <ul className="pb-1">
            {el.children.map((child) => {
              const childName = elementDisplayName(child.id, child.name, t)
              return (
                <li
                  key={child.id}
                  className="flex items-center gap-1.5 rounded-md py-1.5 pr-1.5 pl-8 text-sm text-muted-foreground hover:bg-accent/50 sm:gap-2 sm:pr-2 sm:pl-12"
                  data-testid={`month-income-child-${child.id}`}
                >
                  <EntityIcon name={child.icon} className="text-lg" />
                  <span className="min-w-0 flex-1 truncate" title={childName}>
                    {childName}
                  </span>
                  <span className={PLANNED_COL} />
                  <span className={`${ACTUAL_COL} flex justify-center tabular-nums`}>
                    {actualCell(
                      { id: child.id, type: child.type, name: childName, icon: child.icon, currencyId: el.currencyId, parent: { id: el.id, type: el.type } },
                      child.cells[planMonth?.index ?? 0]?.actual ?? '0',
                      el.currencyId,
                    )}
                  </span>
                  <span className={THIRD_COL} />
                  <span className={SYMBOL_COL} />
                  {actionsColumn ? <span className="w-8 shrink-0" /> : null}
                </li>
              )
            })}
          </ul>
        ) : null}
      </div>
    )
  }

  // the expense table's grouping: folders, then the folder-less rows (named only when
  // there are folders), Uncategorized on its own, then archived rows with money this month
  const incomeBox = (key: string, header: { name: string; planned: string; received: string } | null, rows: PlanCellFigures[]) =>
    rows.length === 0 ? null : (
      <div key={key} className="rounded-md border p-1.5 sm:p-2" data-testid={`month-income-folder-${key}`}>
        {header ? (
          <header className="flex items-center gap-1.5 px-1.5 pb-1 sm:gap-2 sm:px-2">
            <span className="min-w-0 flex-1 truncate text-sm font-medium" title={header.name}>
              {header.name}
            </span>
            <span className={`${PLANNED_COL} text-xs text-muted-foreground tabular-nums`}>{fmt(header.planned)}</span>
            <span className={`${ACTUAL_COL} text-xs text-muted-foreground tabular-nums`}>{future ? EMPTY_CELL : fmt(header.received)}</span>
            <span className={THIRD_COL} />
            <span className={SYMBOL_COL}>{currencyOf(null)?.symbol}</span>
            {actionsColumn ? <span className="w-8 shrink-0" /> : null}
          </header>
        ) : null}
        {rows.map(incomeRow)}
      </div>
    )
  const sumOf = (rows: PlanCellFigures[], pick: (c: PlanCellFigures) => string) =>
    rows.reduce((sum, c) => add(sum, exchangeFn(c.element.currencyId, base, pick(c))), '0')
  const incomeBoxes = (income: PlanMonthFigures['income']) => [
    ...income.folders.map((f) => incomeBox(f.id, { name: f.name, planned: f.planned, received: f.received }, f.rows)),
    incomeBox(
      '__no_folder__',
      income.folders.length > 0
        ? { name: t('budgets.page.budget.structure.no_folder'), planned: sumOf(income.loose, (c) => c.planned), received: sumOf(income.loose, (c) => c.actual) }
        : null,
      income.loose,
    ),
    incomeBox('__uncategorized__', null, income.uncategorized ? [income.uncategorized] : []),
    incomeBox(
      '__archive__',
      { name: t('budgets.page.budget.structure.in_archive'), planned: sumOf(income.archived, (c) => c.planned), received: sumOf(income.archived, (c) => c.actual) },
      income.archived,
    ),
  ]

  const savingsRow = (row: BudgetSavingsElementDto) => (
    <FlowRow
      key={row.id}
      testId={`month-savings-row-${row.id}`}
      icon={row.icon}
      name={row.name}
      muted={row.isArchived === 1}
      planned={renderPlanned({ kind: 'savings', row }, fmt(row.budgeted, row.currencyId))}
      actual={actualCell({ id: row.id, type: BudgetElementType.SAVINGS, name: row.name, icon: row.icon, currencyId: row.currencyId }, row.spent, row.currencyId)}
      third={<span title={t('budgets.page.savings.balance_hint')}>{row.closingBalance !== undefined ? fmt(row.closingBalance, row.currencyId) : EMPTY_CELL}</span>}
      currency={currencyOf(row.currencyId)}
      actionsColumn={actionsColumn}
    />
  )

  const savingsRows = [...(budget.structure.savings ?? [])].sort((a, b) => a.isArchived - b.isArchived || a.position - b.position)
  const savingsSum = totalsWithSavings({ budgeted: '0', spent: '0', available: '0', carry: '0' }, budget, exchangeFn)

  return (
    <>
      {planMonth ? (
        <section className="flex flex-col gap-1" data-testid="month-income">
          <MonthSectionHeader
            foldKey="income"
            testId="month-income-header"
            label={t('budgets.page.plan.section.income')}
            headings={[t('budgets.page.savings.planned'), t('budgets.page.sheet.received'), '']}
            sums={[fmt(planMonth.income.planned), future ? EMPTY_CELL : fmt(planMonth.income.received), '']}
            actionsColumn={actionsColumn}
          />
          {incomeFolded ? null : incomeBoxes(planMonth.income)}
        </section>
      ) : null}
      {savingsRows.length > 0 ? (
        <section className="flex flex-col gap-1" data-testid="month-savings">
          <MonthSectionHeader
            foldKey="savings"
            testId="month-savings-header"
            label={t('budgets.page.plan.section.savings')}
            headings={[t('budgets.page.savings.planned'), t('budgets.page.savings.saved'), t('budgets.page.sheet.balance')]}
            sums={[
              fmt(savingsSum.budgeted),
              future ? EMPTY_CELL : fmt(savingsSum.spent),
              planMonth?.savingsBalance != null ? fmt(planMonth.savingsBalance) : EMPTY_CELL,
            ]}
            actionsColumn={actionsColumn}
          />
          {savingsFolded ? null : <div className="rounded-md border p-1.5 sm:p-2">{savingsRows.map(savingsRow)}</div>}
        </section>
      ) : null}
    </>
  )
}

function TotalLine({ testId, label, value, strong = false, actionsColumn }: { testId: string; label: string; value: string; strong?: boolean; actionsColumn: boolean }) {
  return (
    <div className={`flex items-center gap-1.5 px-3 py-1 sm:gap-2 sm:px-4 ${strong ? 'font-medium' : ''}`} data-testid={testId}>
      <span className={`min-w-0 flex-1 truncate text-sm ${strong ? '' : 'text-muted-foreground'}`}>{label}</span>
      <span className={`${THIRD_COL} text-[15px] tabular-nums`}>{value}</span>
      <span className={SYMBOL_COL} />
      {actionsColumn ? <span className="w-8 shrink-0" /> : null}
    </div>
  )
}

/** The phone totals card's lines under the Total row: what came in, went out and was
 *  set aside this month, then where the savings and the everyday money end up. */
export function MonthTotalsLines({
  budget,
  currencies,
  planMonth,
  expensesSpent,
  future,
  actionsColumn,
}: {
  budget: BudgetDto
  currencies: CurrencyDto[]
  planMonth: PlanMonthFigures | null
  expensesSpent: string
  future: boolean
  actionsColumn: boolean
}) {
  const { t } = useTranslation()
  const base = currencies.find((c) => c.id === budget.meta.currencyId)
  const fmt = (amount: string) => moneyFormat(amount, base, cellOpts(base))
  const hasSavings = (budget.structure.savings ?? []).length > 0
  const savingsSpent = hasSavings ? totalsWithSavings({ budgeted: '0', spent: '0', available: '0', carry: '0' }, budget, makeBudgetExchange(budget, currencies)).spent : null
  const shared = { actionsColumn }
  return (
    <div className="flex flex-col" data-testid="month-totals-lines">
      {planMonth ? (
        <TotalLine testId="month-total-income" label={t('budgets.page.plan.totals.income')} value={future ? EMPTY_CELL : fmt(planMonth.income.received)} {...shared} />
      ) : null}
      <TotalLine testId="month-total-expenses" label={t('budgets.page.plan.totals.expenses')} value={future ? EMPTY_CELL : fmt(expensesSpent)} {...shared} />
      {planMonth && !isZero(planMonth.transfersNet) ? (
        <TotalLine testId="month-total-transfers" label={t('budgets.page.plan.totals.transfers')} value={fmt(planMonth.transfersNet)} {...shared} />
      ) : null}
      {savingsSpent !== null ? (
        <TotalLine testId="month-total-savings" label={t('budgets.page.plan.totals.savings')} value={future ? EMPTY_CELL : fmt(savingsSpent)} {...shared} />
      ) : null}
      {planMonth?.savingsBalance != null ? (
        <TotalLine testId="month-total-savings-balance" label={t('budgets.page.plan.totals.savings_balance')} value={fmt(planMonth.savingsBalance)} {...shared} />
      ) : null}
      {planMonth ? (
        <TotalLine testId="month-total-balance" label={t('budgets.page.phone.balance')} value={fmt(planMonth.balance)} strong {...shared} />
      ) : null}
    </div>
  )
}
