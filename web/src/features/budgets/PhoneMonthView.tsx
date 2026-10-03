import type { ReactNode } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { EntityIcon } from '@/components/EntityIcon'
import { moneyFormat } from '@/lib/money'
import { cmp, isZero } from '@/lib/decimal'
import type { BudgetCommentDto, BudgetDto, BudgetElementDto, BudgetSavingsElementDto, LabelSpendDto } from '@/api/dto/budget'
import { UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import type { BudgetBuckets, FolderBucket } from './budgetMath'
import { budgetTotals, carryOver, displayAvailable, elementDisplayName, makeBudgetExchange, overBudget, rowProgress, totalsWithSavings } from './budgetMath'
import { REPORTING_TAGS_FOLD_ID, useBudgetPeriodStore } from './budgetStore'
import type { BudgetTransactionsTarget } from './BudgetTransactionsDialog'
import type { PlanCellFigures, PlanMonthFigures, SheetTarget } from './phoneMonth'
import { currentMonth } from './planMath'
import { commentCellKey } from './queries'

const INCOME_FOLD_ID = '__phone_income__'
const SAVINGS_FOLD_ID = '__phone_savings__'
const EMPTY = '—'
// name | Budget | Spent: the heading row, folder headers and rows share one grid; the
// Budget column grows to the left for a carry-over lead-in while its right edge stays put
const GRID = 'grid grid-cols-[minmax(0,1fr)_minmax(5.5rem,auto)_5.5rem] items-center gap-x-2'
// A row's figures sit in a subgrid whose right padding only insets the LAST track, so
// every other line keeps its tracks flush right too and pads just its last cell — a
// container padding would pull the Budget column left of the rows' figures
const LAST_CELL = 'pr-2'


export interface PhoneMonthViewProps {
  budget: BudgetDto
  buckets: BudgetBuckets
  currencies: CurrencyDto[]
  selectedDate: string
  /** null while get-budget-plan loads, fails, or still shows another window */
  planMonth: PlanMonthFigures | null
  commentsByCell: Map<string, BudgetCommentDto[]>
  onOpenSheet: (target: SheetTarget) => void
  /** rows whose only action is their transaction list (children, reporting tags) */
  onShowTransactions: (target: BudgetTransactionsTarget) => void
}

interface RowProps {
  testId: string
  icon: string
  name: string
  tag?: string
  /** read-only lead-in to `first`: what earlier months left, e.g. "530.00 +" */
  carry?: { text: string; negative: boolean }
  first: string
  second: string
  secondClass?: string
  progress?: number | null
  barClass?: string
  commented?: boolean
  ariaLabel: string
  onOpen: () => void
  /** an expandable row's name folds and unfolds its children */
  toggle?: { open: boolean; onToggle: () => void }
}

// with a bar under it, the row's first line gives up its bottom padding and minimum
// height so the bar sits right under the figures and the whole row stays 44px tall
const cellPad = (barred: boolean) => (barred ? 'pt-2 pb-1' : 'min-h-11 py-2')
const NAME_CELL = 'flex min-w-0 items-center gap-2 pl-2'

// the name and the figures are separate targets: the name folds an expandable row
// (and does nothing otherwise), only Budget/Spent open the item sheet
function PhoneRow({ testId, icon, name, tag, carry, first, second, secondClass = '', progress = null, barClass = '', commented = false, ariaLabel, onOpen, toggle }: RowProps) {
  const Chevron = toggle?.open ? ChevronDown : ChevronRight
  const pad = cellPad(progress !== null)
  const label = (
    <>
      <span className="min-w-0 truncate text-[15px]">{name}</span>
      {tag ? (
        <span data-testid="phone-currency-tag" className="shrink-0 rounded bg-muted px-1 text-[10px] font-medium text-muted-foreground">
          {tag}
        </span>
      ) : null}
    </>
  )
  return (
    <div className={`${GRID} rounded-md`} data-testid={testId}>
      {toggle ? (
        <button type="button" aria-expanded={toggle.open} onClick={toggle.onToggle} className={`${NAME_CELL} ${pad} rounded-md text-left active:bg-accent/50`}>
          <Chevron aria-hidden="true" className="size-5 shrink-0 text-muted-foreground" />
          {label}
        </button>
      ) : (
        <span className={`${NAME_CELL} ${pad}`}>
          <EntityIcon name={icon} className="text-lg text-muted-foreground" />
          {label}
        </span>
      )}
      <button
        type="button"
        aria-label={ariaLabel}
        onClick={onOpen}
        className={`col-span-2 grid grid-cols-subgrid items-center rounded-md pr-2 active:bg-accent/50 ${pad}`}
      >
        <span className="flex items-baseline justify-end gap-1 text-right text-[15px] tabular-nums">
          {carry ? (
            <span data-testid="phone-carry" className={`shrink-0 text-[13px] ${carry.negative ? 'text-expense' : 'text-muted-foreground'}`}>
              {carry.text}
            </span>
          ) : null}
          <span className="shrink-0">{first}</span>
        </span>
        <span className={`relative text-right text-[15px] tabular-nums ${secondClass}`}>
          {second}
          {commented ? (
            <span
              aria-hidden="true"
              data-testid="phone-comment-indicator"
              className="absolute -top-1.5 -right-2 h-0 w-0 border-t-[7px] border-l-[7px] border-t-primary border-l-transparent"
            />
          ) : null}
        </span>
      </button>
      {progress !== null ? (
        // the bar spans the row, not the name column: a carry-over widens the Budget
        // column per row, which would leave every bar a different length
        <span data-testid="phone-progress" className="col-span-full mx-2 mb-2 h-1 overflow-hidden rounded-full bg-muted">
          <span className={`block h-full rounded-full ${barClass}`} style={{ width: `${Math.round(progress * 100)}%` }} />
        </span>
      ) : null}
    </div>
  )
}

function Card({ testId, header, children }: { testId: string; header?: ReactNode; children: ReactNode }) {
  return (
    <section className="rounded-md border p-1" data-testid={testId}>
      {header}
      {children}
    </section>
  )
}

// a section's column labels, above its cards: the section name stands where the
// currency code used to, and each section names its own two figures
function SectionHeading({ testId, name, first, second, className = '' }: { testId: string; name: string; first: string; second: string; className?: string }) {
  return (
    // outside the cards: pr-[5px] is a card's border + padding, so the tracks end where the rows' do
    <div className={`${GRID} pr-[5px] pl-3 text-[11px] uppercase tracking-wide text-muted-foreground ${className}`} data-testid={testId}>
      <span className="truncate">{name}</span>
      <span className="text-right">{first}</span>
      <span className={`text-right ${LAST_CELL}`}>{second}</span>
    </div>
  )
}

/** a folded section's one line: Total and its two sums, the same name | Budget |
 *  Spent geometry as a folder header, as one fold button */
function SectionSummary({
  testId,
  open,
  onToggle,
  label,
  first,
  second,
  firstTestId,
  secondTestId,
}: {
  testId: string
  open: boolean
  onToggle: () => void
  label: string
  first: string
  second: string
  firstTestId?: string
  secondTestId?: string
}) {
  const Chevron = open ? ChevronDown : ChevronRight
  return (
    <button
      type="button"
      data-testid={testId}
      aria-expanded={open}
      onClick={onToggle}
      className={`${GRID} min-h-11 w-full rounded-md py-1.5 pl-2 text-left text-xs font-medium text-muted-foreground active:bg-accent/50`}
    >
      <span className="flex min-w-0 items-center gap-1">
        <Chevron className="size-4 shrink-0" />
        <span className="truncate">{label}</span>
      </span>
      <span data-testid={firstTestId} className="text-right tabular-nums">
        {first}
      </span>
      <span data-testid={secondTestId} className={`text-right tabular-nums ${LAST_CELL}`}>
        {second}
      </span>
    </button>
  )
}

function CardHeader({ name, first, second }: { name: string; first?: string; second?: string }) {
  return (
    <div className={`${GRID} pt-1.5 pb-0.5 pl-2 text-xs font-medium text-muted-foreground`}>
      <span className="truncate">{name}</span>
      <span className="text-right tabular-nums">{first}</span>
      <span className={`text-right tabular-nums ${LAST_CELL}`}>{second}</span>
    </div>
  )
}

function TotalLine({ testId, label, value, negative = false }: { testId: string; label: string; value: string; negative?: boolean }) {
  return (
    <div className="flex items-baseline justify-between gap-3" data-testid={testId}>
      <span className="text-[13px] text-muted-foreground">{label}</span>
      <span className={`text-[15px] tabular-nums ${negative ? 'text-expense' : ''}`}>{value}</span>
    </div>
  )
}

export function PhoneMonthView({ budget, buckets, currencies, selectedDate, planMonth, commentsByCell, onOpenSheet, onShowTransactions }: PhoneMonthViewProps) {
  const { t } = useTranslation()
  const unfolded = useBudgetPeriodStore((s) => s.unfoldedElements)
  const toggleElement = useBudgetPeriodStore((s) => s.toggleElement)

  const base = budget.meta.currencyId
  const currencyOf = (id: Id | null) => currencies.find((c) => c.id === (id ?? base))
  const fmt = (amount: string, currencyId: Id | null = base) => {
    const c = currencyOf(currencyId)
    return moneyFormat(amount, c, { showCurrency: false, useNativePrecision: false, maxPrecision: c?.fractionDigits ?? 2 })
  }
  const tagOf = (currencyId: Id | null) => (currencyId && currencyId !== base ? currencyOf(currencyId)?.code : undefined)
  const future = selectedDate > currentMonth()
  const commented = (id: Id) => (commentsByCell.get(commentCellKey(id, selectedDate))?.length ?? 0) > 0

  const expenseRow = (element: BudgetElementDto) => {
    const name = elementDisplayName(element.id, element.name, t)
    const isUncategorized = element.id === UNCATEGORIZED_ID
    const carry = carryOver(element)
    const figures = { budgeted: element.budgeted, spent: element.spent, available: displayAvailable(element), carry }
    const overspent = !isUncategorized && overBudget(figures, future)
    const carryText = isUncategorized || isZero(carry) ? null : `${fmt(carry, element.currencyId)} +`
    const budgetText = isUncategorized ? EMPTY : fmt(element.budgeted, element.currencyId)
    const spentText = future ? EMPTY : fmt(element.spent, element.currencyId)
    const expandable = element.children.length > 0
    const open = !!unfolded[element.id]
    return (
      <div key={element.id}>
        <PhoneRow
          testId={`phone-row-${element.id}`}
          icon={element.icon}
          name={name}
          tag={tagOf(element.currencyId)}
          carry={carryText ? { text: carryText, negative: cmp(carry, '0') < 0 } : undefined}
          first={budgetText}
          second={spentText}
          secondClass={overspent ? 'text-expense' : ''}
          // a row with nothing to measure against still draws the empty track, as a
          // budgeted row with nothing spent does
          progress={isUncategorized || future ? null : (rowProgress(figures) ?? 0)}
          barClass={overspent ? 'bg-expense' : 'bg-muted-foreground/40'}
          commented={commented(element.id)}
          ariaLabel={t('budgets.page.phone.row_aria', { name, budget: carryText ? `${carryText} ${budgetText}` : budgetText, spent: spentText })}
          onOpen={() => onOpenSheet({ kind: 'expense', element })}
          toggle={expandable ? { open, onToggle: () => toggleElement(element.id) } : undefined}
        />
        {expandable && open
          ? element.children.map((child) => {
              const childName = elementDisplayName(child.id, child.name, t)
              const childSpent = future ? EMPTY : fmt(child.spent, element.currencyId)
              return (
                <div key={child.id} data-testid={`phone-child-${child.id}`} className={`${GRID} rounded-md text-sm text-muted-foreground`}>
                  <span className="flex min-h-10 min-w-0 items-center gap-2 py-1.5 pl-9">
                    <EntityIcon name={child.icon} className="text-lg" />
                    <span className="truncate">{childName}</span>
                  </span>
                  <button
                    type="button"
                    aria-label={t('budgets.page.phone.child_aria', { name: childName, spent: childSpent })}
                    className="col-span-2 grid min-h-10 grid-cols-subgrid items-center rounded-md py-1.5 pr-2 active:bg-accent/50"
                    onClick={() =>
                      onShowTransactions({
                        id: child.id,
                        type: child.type,
                        name: childName,
                        icon: child.icon,
                        currencyId: element.currencyId,
                        parent: { id: element.id, type: element.type },
                      })
                    }
                  >
                    <span />
                    <span className="text-right tabular-nums">{childSpent}</span>
                  </button>
                </div>
              )
            })
          : null}
      </div>
    )
  }

  const folderCard = (key: string, name: string | null, bucket: FolderBucket) => (
    <Card
      key={key}
      testId={`phone-folder-${key}`}
      header={name !== null ? <CardHeader name={name} first={fmt(bucket.stats.budgeted)} second={future ? EMPTY : fmt(bucket.stats.spent)} /> : undefined}
    >
      {bucket.elements.map(expenseRow)}
    </Card>
  )

  // income and savings fill toward their plan and turn green once it is met: unlike
  // spending, reaching the figure is the goal, so only the bar is coloured
  const planBar = (planned: string, actual: string) => ({
    progress: future ? null : (rowProgress({ budgeted: planned, spent: actual }) ?? 0),
    barClass: cmp(planned, '0') > 0 && cmp(actual, planned) >= 0 ? 'bg-income' : 'bg-muted-foreground/40',
  })

  const incomeRow = (row: PlanCellFigures) => {
    const el = row.element
    const name = elementDisplayName(el.id, el.name, t)
    const planned = fmt(row.planned, el.currencyId)
    const received = future ? EMPTY : fmt(row.actual, el.currencyId)
    return (
      <PhoneRow
        key={`${el.id}:${el.type}`}
        testId={`phone-income-row-${el.id}`}
        icon={el.icon}
        name={name}
        tag={tagOf(el.currencyId)}
        first={planned}
        second={received}
        {...planBar(row.planned, row.actual)}
        commented={commented(el.id)}
        ariaLabel={t('budgets.page.phone.income_row_aria', { name, planned, received })}
        onOpen={() => onOpenSheet({ kind: 'plan', cell: row })}
      />
    )
  }

  const savingsRow = (row: BudgetSavingsElementDto) => {
    const planned = fmt(row.budgeted, row.currencyId)
    const saved = future ? EMPTY : fmt(row.spent, row.currencyId)
    return (
      <PhoneRow
        key={row.id}
        testId={`phone-savings-row-${row.id}`}
        icon={row.icon}
        name={row.name}
        tag={tagOf(row.currencyId)}
        first={planned}
        second={saved}
        {...planBar(row.budgeted, row.spent)}
        commented={commented(row.id)}
        ariaLabel={t('budgets.page.phone.savings_row_aria', { name: row.name, planned, saved })}
        onOpen={() => onOpenSheet({ kind: 'savings', row })}
      />
    )
  }

  const labelRow = (label: LabelSpendDto) => {
    const spent = future ? EMPTY : fmt(label.spent)
    return (
      <div key={label.id} data-testid={`phone-label-${label.id}`} className={`${GRID} rounded-md`}>
        <span className={`${NAME_CELL} ${cellPad(false)}`}>
          <EntityIcon name={label.icon} className="text-lg text-muted-foreground" />
          <span className="truncate text-[15px]">{label.name}</span>
        </span>
        <button
          type="button"
          aria-label={t('budgets.page.phone.child_aria', { name: label.name, spent })}
          className="col-span-2 grid min-h-11 grid-cols-subgrid items-center rounded-md py-2 pr-2 active:bg-accent/50"
          onClick={() => onShowTransactions({ id: label.id, type: 'label', name: label.name, icon: label.icon, currencyId: null })}
        >
          <span className="text-right text-[15px] text-muted-foreground">{EMPTY}</span>
          <span className="text-right text-[15px] tabular-nums">{spent}</span>
        </button>
      </div>
    )
  }

  const exchangeFn = makeBudgetExchange(budget, currencies)
  const expenseTotals = budgetTotals(buckets)
  const savingsRows = [...(budget.structure.savings ?? [])].sort((a, b) => a.isArchived - b.isArchived || a.position - b.position)
  const savingsSum = savingsRows.length > 0 ? totalsWithSavings({ budgeted: '0', spent: '0', available: '0', carry: '0' }, budget, exchangeFn) : null
  const labels = budget.structure.labels ?? []
  const labelsOpen = !!unfolded[REPORTING_TAGS_FOLD_ID]
  const incomeOpen = !!unfolded[INCOME_FOLD_ID]
  const savingsOpen = !!unfolded[SAVINGS_FOLD_ID]
  const hasFolders = buckets.withFolder.length > 0
  const showFlows = planMonth !== null || savingsSum !== null

  return (
    <div className="flex flex-col gap-3" data-testid="phone-month-view">
      {showFlows ? (
        // income and savings share one card under one Planned · Actual heading: a
        // line each, folded by default, so the money coming in and set aside costs
        // two rows above the expenses
        <>
          <SectionHeading testId="phone-heading-flows" name={currencyOf(base)?.code ?? ''} first={t('budgets.page.savings.planned')} second={t('budgets.page.phone.actual')} />
          <Card testId="phone-flows">
            {planMonth ? (
              <>
                <SectionSummary
                  testId="phone-income-summary"
                  open={incomeOpen}
                  onToggle={() => toggleElement(INCOME_FOLD_ID)}
                  label={t('budgets.page.plan.section.income')}
                  first={fmt(planMonth.income.planned)}
                  second={future ? EMPTY : fmt(planMonth.income.received)}
                  firstTestId="phone-income-planned"
                  secondTestId="phone-income-received"
                />
                {incomeOpen ? planMonth.income.rows.map(incomeRow) : null}
              </>
            ) : null}
            {savingsSum ? (
              <>
                <SectionSummary
                  testId="phone-savings-summary"
                  open={savingsOpen}
                  onToggle={() => toggleElement(SAVINGS_FOLD_ID)}
                  label={t('budgets.page.savings.title')}
                  first={fmt(savingsSum.budgeted)}
                  second={future ? EMPTY : fmt(savingsSum.spent)}
                />
                {savingsOpen ? savingsRows.map(savingsRow) : null}
              </>
            ) : null}
          </Card>
        </>
      ) : null}
      <SectionHeading
        className={showFlows ? 'mt-3' : ''}
        testId="phone-heading-expenses"
        name={t('budgets.page.plan.totals.expenses')}
        first={t('budgets.page.budget.structure.tab.budgeted')}
        second={t('budgets.page.budget.structure.tab.spent')}
      />
      {buckets.withFolder.filter((b) => b.elements.length > 0).map((b) => folderCard(b.folder!.id, b.folder!.name, b))}
      {buckets.withoutFolder.elements.length > 0
        ? folderCard('__no_folder__', hasFolders ? t('budgets.page.plan.menu.no_folder') : null, buckets.withoutFolder)
        : null}
      {buckets.uncategorized.elements.length > 0 ? folderCard('__uncategorized__', null, buckets.uncategorized) : null}
      {labels.length > 0 ? (
        <Card testId="phone-labels">
          <button
            type="button"
            aria-expanded={labelsOpen}
            onClick={() => toggleElement(REPORTING_TAGS_FOLD_ID)}
            className="flex min-h-11 w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm font-medium"
          >
            {labelsOpen ? <ChevronDown className="size-4 shrink-0 text-muted-foreground" /> : <ChevronRight className="size-4 shrink-0 text-muted-foreground" />}
            {t('budgets.page.budget.structure.labels.heading')}
          </button>
          {labelsOpen ? labels.map(labelRow) : null}
        </Card>
      ) : null}
      {buckets.archive.elements.length > 0 ? folderCard('__archive__', t('budgets.page.budget.structure.in_archive'), buckets.archive) : null}


      <section
        className="mb-[max(env(safe-area-inset-bottom),0.75rem)] flex flex-col gap-2 rounded-md border px-3 py-2.5"
        data-testid="phone-totals"
      >
        <span className="text-[15px] font-medium">{t('budgets.page.budget.structure.total.name')}</span>
        <div className="flex items-baseline justify-between gap-3" data-testid="phone-total-budget">
          <span className="text-[13px] text-muted-foreground">{t('budgets.page.budget.structure.tab.budgeted')}</span>
          <span className="flex flex-col items-end">
            <span className="text-[15px] tabular-nums">
              {isZero(expenseTotals.carry) ? (
                fmt(expenseTotals.budgeted)
              ) : (
                <>
                  <span className={`text-[13px] ${cmp(expenseTotals.carry, '0') < 0 ? 'text-expense' : 'text-muted-foreground'}`}>
                    {fmt(expenseTotals.carry)} +
                  </span>{' '}
                  {fmt(expenseTotals.budgeted)}
                </>
              )}
            </span>
            <span
              data-testid="phone-budget-available"
              className={`text-[13px] tabular-nums ${cmp(expenseTotals.available, '0') < 0 ? 'text-expense' : 'text-muted-foreground'}`}
            >
              {t('budgets.page.phone.available', { amount: fmt(expenseTotals.available) })}
            </span>
          </span>
        </div>
        {planMonth ? (
          <TotalLine
            testId="phone-total-income"
            label={t('budgets.page.plan.totals.income')}
            value={future ? EMPTY : fmt(planMonth.income.received)}
          />
        ) : null}
        <TotalLine testId="phone-total-expenses" label={t('budgets.page.plan.totals.expenses')} value={future ? EMPTY : fmt(expenseTotals.spent)} />
        {planMonth && !isZero(planMonth.transfersNet) ? (
          <TotalLine testId="phone-total-transfers" label={t('budgets.page.plan.totals.transfers')} value={fmt(planMonth.transfersNet)} />
        ) : null}
        {savingsSum ? <TotalLine testId="phone-total-savings" label={t('budgets.page.plan.totals.savings')} value={future ? EMPTY : fmt(savingsSum.spent)} /> : null}
        {planMonth?.savingsBalance != null ? (
          <TotalLine testId="phone-total-savings-balance" label={t('budgets.page.plan.totals.savings_balance')} value={fmt(planMonth.savingsBalance)} />
        ) : null}
        {planMonth ? <TotalLine testId="phone-total-balance" label={t('budgets.page.phone.balance')} value={fmt(planMonth.balance)} /> : null}
      </section>
    </div>
  )
}
