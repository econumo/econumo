import { useTranslation } from 'react-i18next'
import { Pencil } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { EntityIcon } from '@/components/EntityIcon'
import { moneyFormat } from '@/lib/money'
import { cmp, sub } from '@/lib/decimal'
import type { BudgetCommentDto } from '@/api/dto/budget'
import { BudgetElementType, isIncomeType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { carryOver, displayAvailable, elementDisplayName, rowState } from './budgetMath'
import { sortByCreatedAt } from './CommentThread'
import type { SheetTarget } from './phoneMonth'
import { sheetCell, sheetIcon, sheetSetsPlan } from './phoneMonth'
import { currentMonth, formatPlanMonth } from './planMath'

const EMPTY = '—'

export interface ElementSheetProps {
  target: SheetTarget | null
  /** the month the sheet describes, 'YYYY-MM-01' */
  month: string
  baseCurrencyId: Id
  currencies: CurrencyDto[]
  /** converts at `month`'s rates */
  exchange: (from: Id, to: Id, amount: string) => string
  comments: BudgetCommentDto[]
  commentsReadOnly: boolean
  canSetAmount: boolean
  onClose: () => void
  onSetAmount: () => void
  onOpenComments: () => void
  /** absent: no transaction list for this target */
  onShowTransactions?: () => void
  /** absent: nothing to edit (Uncategorized) */
  onEdit?: () => void
  /** false keeps the edit button visible but inactive */
  canEdit?: boolean
}

interface Figure {
  key: string
  label: string
  value: string
  negative?: boolean
}

export function ElementSheet({
  target,
  month,
  baseCurrencyId,
  currencies,
  exchange,
  comments,
  commentsReadOnly,
  canSetAmount,
  onClose,
  onSetAmount,
  onOpenComments,
  onShowTransactions,
  onEdit,
  canEdit = false,
}: ElementSheetProps) {
  const { t, i18n } = useTranslation()
  if (!target) {
    return null
  }
  const cell = sheetCell(target, baseCurrencyId)
  const name = elementDisplayName(cell.id, cell.name, t)
  const monthLabel = formatPlanMonth(month, i18n.language)
  const currency = currencies.find((c) => c.id === cell.currencyId)
  const baseCurrency = currencies.find((c) => c.id === baseCurrencyId)
  const foreign = cell.currencyId !== baseCurrencyId
  const future = month > currentMonth()
  const isUncategorized = cell.id === UNCATEGORIZED_ID
  // a reporting tag is not a budget cell: no comment thread
  const hasThread = !isUncategorized && target.kind !== 'label'
  const fmtIn = (amount: string, c: CurrencyDto | undefined) =>
    moneyFormat(amount, c, { showCurrency: false, useNativePrecision: false, maxPrecision: c?.fractionDigits ?? 2 })
  // the sheet repeats a foreign item's code beside every amount (the row only tags its name)
  const fmt = (amount: string) => `${fmtIn(amount, currency)}${foreign && currency ? ` ${currency.code}` : ''}`
  const actual = (amount: string) => (future ? EMPTY : fmt(amount))
  const label = {
    budget: t('budgets.page.budget.structure.tab.budgeted'),
    spent: t('budgets.page.budget.structure.tab.spent'),
    available: t('budgets.page.budget.structure.tab.available'),
    planned: t('budgets.page.savings.planned'),
    received: t('budgets.page.sheet.received'),
    saved: t('budgets.page.savings.saved'),
    balance: t('budgets.page.sheet.balance'),
  }

  const figures: Figure[] = []
  let stateSentence: string | null = null
  let actualInBase: string
  if (target.kind === 'expense') {
    const el = target.element
    const available = displayAvailable(el)
    if (!isUncategorized) {
      figures.push({ key: 'budget', label: label.budget, value: fmt(el.budgeted) })
    }
    figures.push({ key: 'spent', label: label.spent, value: actual(el.spent) })
    if (!isUncategorized) {
      figures.push({ key: 'available', label: label.available, value: fmt(available), negative: cmp(available, '0') < 0 })
      const state = rowState({ budgeted: el.budgeted, spent: el.spent, available }, future)
      if (state === 'covered') {
        stateSentence = t('budgets.page.sheet.covered', { over: fmt(sub(el.spent, el.budgeted)), carry: fmt(carryOver(el)) })
      }
    }
    actualInBase = el.budgetSpent
  } else if (target.kind === 'label') {
    figures.push({ key: 'spent', label: label.spent, value: actual(target.label.spent) })
    actualInBase = target.label.spent
  } else {
    const planned = target.kind === 'savings' ? target.row.budgeted : target.cell.planned
    const done = target.kind === 'savings' ? target.row.spent : target.cell.actual
    const closing = target.kind === 'savings' ? target.row.closingBalance : target.cell.closingBalance
    const type = target.kind === 'savings' ? BudgetElementType.SAVINGS : target.cell.element.type
    if (type === BudgetElementType.SAVINGS) {
      figures.push({ key: 'planned', label: label.planned, value: fmt(planned) })
      figures.push({ key: 'saved', label: label.saved, value: actual(done) })
      figures.push({ key: 'balance', label: label.balance, value: closing !== undefined ? fmt(closing) : EMPTY })
    } else if (isIncomeType(type)) {
      figures.push({ key: 'planned', label: label.planned, value: fmt(planned) })
      figures.push({ key: 'received', label: label.received, value: actual(done) })
    } else {
      figures.push({ key: 'budget', label: label.budget, value: fmt(planned) })
      figures.push({ key: 'spent', label: label.spent, value: actual(done) })
    }
    actualInBase = foreign ? exchange(cell.currencyId, baseCurrencyId, done) : done
  }

  // the two most recent, oldest first, as the desktop hover preview shows them
  const latest = sortByCreatedAt(comments).slice(-2)
  const canComment = hasThread && !(commentsReadOnly && comments.length === 0)
  const rate = foreign ? exchange(baseCurrencyId, cell.currencyId, '1') : null

  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={`${name} · ${monthLabel}`}
      titleIcon={<EntityIcon name={sheetIcon(target)} className="text-xl leading-none text-muted-foreground" />}
      headerAction={
        onEdit ? (
          <Button type="button" variant="ghost" size="icon-sm" aria-label={t('common.button.edit.label')} disabled={!canEdit} onClick={onEdit}>
            <Pencil className="size-4" />
          </Button>
        ) : undefined
      }
    >
      <div className="flex flex-col gap-4" data-testid="element-sheet">
        <div className={`grid gap-2 ${figures.length === 1 ? 'grid-cols-1' : figures.length === 2 ? 'grid-cols-2' : 'grid-cols-3'}`}>
          {figures.map((f) => (
            <div key={f.key} className="flex flex-col items-center text-center" data-testid={`sheet-figure-${f.key}`}>
              <span className="text-[13px] text-muted-foreground">{f.label}</span>
              <span className={`text-[17px] font-medium tabular-nums ${f.negative ? 'text-expense' : ''}`}>{f.value}</span>
            </div>
          ))}
        </div>
        {stateSentence ? (
          <p className="text-sm" data-testid="sheet-state">
            {stateSentence}
          </p>
        ) : null}
        {foreign && baseCurrency ? (
          <div className="flex flex-col gap-0.5 text-xs text-muted-foreground">
            {!future ? (
              <span data-testid="sheet-converted">
                {t('budgets.page.sheet.converted', { amount: fmtIn(actualInBase, baseCurrency), currency: baseCurrency.code })}
              </span>
            ) : null}
            {rate !== null && currency ? (
              <span data-testid="sheet-rate">
                {t('budgets.modal.expense_widget.conversion_rate', {
                  period: monthLabel,
                  defaultCurrency: baseCurrency.code,
                  rate: `${moneyFormat(rate, null, { showCurrency: false, useNativePrecision: false, maxPrecision: 4 })} ${currency.code}`,
                })}
              </span>
            ) : null}
          </div>
        ) : null}
        {hasThread ? (
          <div className="flex items-start gap-2 border-t pt-3">
            {latest.length > 0 ? (
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                {latest.map((c) => (
                  <p key={c.id} className="truncate text-sm" data-testid="sheet-comment">
                    <span className="font-medium">{c.author.name}</span> <span className="text-muted-foreground">{c.comment}</span>
                  </p>
                ))}
              </div>
            ) : (
              <p className="min-w-0 flex-1 text-sm text-muted-foreground" data-testid="sheet-no-comments">
                {t('budgets.page.plan.comments.empty')}
              </p>
            )}
            {canComment ? (
              <button type="button" className="shrink-0 text-sm font-medium text-primary hover:underline" onClick={onOpenComments}>
                {comments.length > 0 ? t('budgets.page.plan.comments.disclosure', { count: comments.length }) : t('budgets.page.plan.comments.add')}
              </button>
            ) : null}
          </div>
        ) : null}
        {canSetAmount || onShowTransactions ? (
          <div className="flex gap-3 [&>button]:h-11 [&>button]:flex-1">
            {/* the primary action on the right, as in every other dialog's action row */}
            {onShowTransactions ? (
              <Button type="button" variant="secondary" onClick={onShowTransactions}>
                {t('budgets.page.sheet.transactions')}
              </Button>
            ) : null}
            {canSetAmount ? (
              <Button type="button" onClick={onSetAmount}>
                {sheetSetsPlan(target) ? t('budgets.page.sheet.set_plan') : t('budgets.modal.set_limit_form.header')}
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>
    </ResponsiveDialog>
  )
}
