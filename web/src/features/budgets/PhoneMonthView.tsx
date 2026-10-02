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
  /** an expandable row's chevron: a separate button over the icon slot */
  toggle?: { open: boolean; onToggle: () => void; label: string }
}

function PhoneRow({ testId, icon, name, tag, carry, first, second, secondClass = '', progress = null, barClass = '', commented = false, ariaLabel, onOpen, toggle }: RowProps) {
  const Chevron = toggle?.open ? ChevronDown : ChevronRight
  return (
    <div className="relative" data-testid={testId}>
      <button type="button" aria-label={ariaLabel} onClick={onOpen} className={`${GRID} min-h-11 w-full rounded-md px-2 py-2 text-left active:bg-accent/50`}>
        <span className="flex min-w-0 flex-col gap-1">
          <span className="flex min-w-0 items-center gap-2">
            {toggle ? <span className="size-5 shrink-0" /> : <EntityIcon name={icon} className="text-lg text-muted-foreground" />}
            <span className="min-w-0 truncate text-[15px]">{name}</span>
            {tag ? (
              <span data-testid="phone-currency-tag" className="shrink-0 rounded bg-muted px-1 text-[10px] font-medium text-muted-foreground">
                {tag}
              </span>
            ) : null}
          </span>
        </span>
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
        {progress !== null ? (
          // the bar spans the row, not the name column: a carry-over widens the Budget
          // column per row, which would leave every bar a different length
          <span data-testid="phone-progress" className="col-span-full mt-1 ml-7 h-1 overflow-hidden rounded-full bg-muted">
            <span className={`block h-full rounded-full ${barClass}`} style={{ width: `${Math.round(progress * 100)}%` }} />
          </span>
        ) : null}
      </button>
      {toggle ? (
        // a sibling, never nested in the row button: the chevron folds, the row opens the sheet
        <button
          type="button"
          aria-expanded={toggle.open}
          aria-label={toggle.label}
          onClick={toggle.onToggle}
          className="absolute top-0 -left-1 flex size-11 items-center justify-center text-muted-foreground"
        >
          <Chevron className="size-4.5" />
        </button>
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
function SectionHeading({ testId, name, first, second }: { testId: string; name: string; first: string; second: string }) {
  return (
    <div className={`${GRID} px-3 text-[11px] uppercase tracking-wide text-muted-foreground`} data-testid={testId}>
      <span className="truncate">{name}</span>
      <span className="text-right">{first}</span>
      <span className="text-right">{second}</span>
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
      className={`${GRID} min-h-11 w-full rounded-md px-2 py-1.5 text-left text-xs font-medium text-muted-foreground active:bg-accent/50`}
    >
      <span className="flex min-w-0 items-center gap-1">
        <Chevron className="size-4 shrink-0" />
        <span className="truncate">{label}</span>
      </span>
      <span data-testid={firstTestId} className="text-right tabular-nums">
        {first}
      </span>
      <span data-testid={secondTestId} className="text-right tabular-nums">
        {second}
      </span>
    </button>
  )
}

function CardHeader({ name, first, second }: { name: string; first?: string; second?: string }) {
  return (
    <div className={`${GRID} px-2 pt-1.5 pb-0.5 text-xs font-medium text-muted-foreground`}>
      <span className="truncate">{name}</span>
      <span className="text-right tabular-nums">{first}</span>
      <span className="text-right tabular-nums">{second}</span>
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
  const expandLabel = (open: boolean) => t(open ? 'common.button.collapse.label' : 'common.button.expand.label')

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
          progress={isUncategorized ? null : rowProgress(figures, future)}
          barClass={overspent ? 'bg-expense' : 'bg-muted-foreground/40'}
          commented={commented(element.id)}
          ariaLabel={t('budgets.page.phone.row_aria', { name, budget: carryText ? `${carryText} ${budgetText}` : budgetText, spent: spentText })}
          onOpen={() => onOpenSheet({ kind: 'expense', element })}
          toggle={expandable ? { open, onToggle: () => toggleElement(element.id), label: expandLabel(open) } : undefined}
        />
        {expandable && open
          ? element.children.map((child) => {
              const childName = elementDisplayName(child.id, child.name, t)
              return (
                <button
                  key={child.id}
                  type="button"
                  data-testid={`phone-child-${child.id}`}
                  className={`${GRID} min-h-10 w-full rounded-md py-1.5 pr-2 pl-9 text-left text-sm text-muted-foreground active:bg-accent/50`}
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
                  <span className="flex min-w-0 items-center gap-2">
                    <EntityIcon name={child.icon} className="text-lg" />
                    <span className="truncate">{childName}</span>
                  </span>
                  <span />
                  <span className="text-right tabular-nums">{future ? EMPTY : fmt(child.spent, element.currencyId)}</span>
                </button>
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
        commented={commented(row.id)}
        ariaLabel={t('budgets.page.phone.savings_row_aria', { name: row.name, planned, saved })}
        onOpen={() => onOpenSheet({ kind: 'savings', row })}
      />
    )
  }

  const labelRow = (label: LabelSpendDto) => (
    <button
      key={label.id}
      type="button"
      data-testid={`phone-label-${label.id}`}
      className={`${GRID} min-h-11 w-full rounded-md px-2 py-2 text-left active:bg-accent/50`}
      onClick={() => onShowTransactions({ id: label.id, type: 'label', name: label.name, icon: label.icon, currencyId: null })}
    >
      <span className="flex min-w-0 items-center gap-2">
        <EntityIcon name={label.icon} className="text-lg text-muted-foreground" />
        <span className="truncate text-[15px]">{label.name}</span>
      </span>
      <span className="text-right text-[15px] text-muted-foreground">{EMPTY}</span>
      <span className="text-right text-[15px] tabular-nums">{future ? EMPTY : fmt(label.spent)}</span>
    </button>
  )

  const exchangeFn = makeBudgetExchange(budget, currencies)
  const expenseTotals = budgetTotals(buckets)
  const savingsRows = [...(budget.structure.savings ?? [])].sort((a, b) => a.isArchived - b.isArchived || a.position - b.position)
  const savingsSum = savingsRows.length > 0 ? totalsWithSavings({ budgeted: '0', spent: '0', available: '0', carry: '0' }, budget, exchangeFn) : null
  const labels = budget.structure.labels ?? []
  const labelsOpen = !!unfolded[REPORTING_TAGS_FOLD_ID]
  const incomeOpen = !!unfolded[INCOME_FOLD_ID]
  const savingsOpen = !!unfolded[SAVINGS_FOLD_ID]
  const hasFolders = buckets.withFolder.length > 0

  return (
    <div className="flex flex-col gap-3" data-testid="phone-month-view">
      {planMonth ? (
        <SectionHeading
          testId="phone-heading-income"
          name={t('budgets.page.plan.section.income')}
          first={t('budgets.page.savings.planned')}
          second={t('budgets.page.sheet.received')}
        />
      ) : null}
      {planMonth ? (
        <Card testId="phone-income">
          <SectionSummary
            testId="phone-income-summary"
            open={incomeOpen}
            onToggle={() => toggleElement(INCOME_FOLD_ID)}
            label={t('budgets.page.budget.structure.total.name')}
            first={fmt(planMonth.income.planned)}
            second={future ? EMPTY : fmt(planMonth.income.received)}
            firstTestId="phone-income-planned"
            secondTestId="phone-income-received"
          />
          {incomeOpen ? planMonth.income.rows.map(incomeRow) : null}
        </Card>
      ) : null}

      {savingsRows.length > 0 && savingsSum ? (
        <>
          <SectionHeading
            testId="phone-heading-savings"
            name={t('budgets.page.savings.title')}
            first={t('budgets.page.savings.planned')}
            second={t('budgets.page.savings.saved')}
          />
          <Card testId="phone-savings">
            <SectionSummary
              testId="phone-savings-summary"
              open={savingsOpen}
              onToggle={() => toggleElement(SAVINGS_FOLD_ID)}
              label={t('budgets.page.budget.structure.total.name')}
              first={fmt(savingsSum.budgeted)}
              second={future ? EMPTY : fmt(savingsSum.spent)}
            />
            {savingsOpen ? savingsRows.map(savingsRow) : null}
          </Card>
        </>
      ) : null}
      <SectionHeading
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
