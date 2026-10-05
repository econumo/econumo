import type { ReactElement, ReactNode } from 'react'
import { ChevronDown, ChevronRight, Info } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { EntityIcon } from '@/components/EntityIcon'
import { cmp, isZero } from '@/lib/decimal'
import { moneyFormat } from '@/lib/money'
import type { MoneyFormatOptions } from '@/lib/money'
import type { BudgetChildElementDto, BudgetDto, BudgetElementDto, LabelSpendDto } from '@/api/dto/budget'
import { UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { UserDto } from '@/api/dto/user'
import { useCurrencies } from '@/features/currencies/queries'
import { COMMENT_ANCHOR_ATTR, commentAnchorOf } from './cellDom'
import { isEnvelopeType } from './elementEdit'
import type { BudgetBuckets, BucketStats, FolderBucket } from './budgetMath'
import { budgetTotals, carryOver, displayAvailable, elementDisplayName, nothingToShow, overBudget } from './budgetMath'
import { REPORTING_TAGS_FOLD_ID, useBudgetPeriodStore } from './budgetStore'
import type { BudgetTransactionsTarget } from './BudgetTransactionsDialog'
import { ActionsSpacer, CurrencyTag, Dash, FolderLine, MonthSectionHeader, RowMenu } from './monthLines'
import type { MenuAction } from './monthLayout'
import { CHILD_INDENT, FIRST_COL, FOLD_LINE, foldOnLineClick, FOLDER_INDENT, LINE, NAME_COL, ROW_INDENT, RowLevelContext, SECOND_COL, THIRD_COL, useRowLevel } from './monthLayout'

export interface ElementRowExtras {
  /** the budget cell contents (set-limit editor) — defaults to a plain value */
  renderBudgetCell?: (element: BudgetElementDto) => ReactNode
  /** the entry point of a cell without `renderBudgetCell` that is not editable
   *  here (non-editable role, an archived element, or a read-only section): the
   *  plain budgeted value opens the cell's thread */
  onBudgetCellComments?: (element: BudgetElementDto, anchor: HTMLElement) => void
  /** touch viewports: the budgeted amount opens the item sheet; wins over
   *  `renderBudgetCell` and `onBudgetCellComments` */
  onBudgetCellDetails?: (element: BudgetElementDto) => void
  /** wraps the budgeted cell (hover preview, Shift+F2) */
  wrapBudgetCell?: (element: BudgetElementDto, cell: ReactElement) => ReactNode
  /** the comment-marker overlay for the budgeted cell — absolutely positioned by
   *  the caller; returns null/undefined for a cell with no comments */
  renderBudgetCellMarker?: (element: BudgetElementDto) => ReactNode
  /** trailing actions (edit-mode menus, drag handle) */
  renderActions?: (element: BudgetElementDto, bucket: FolderBucket) => ReactNode
  renderRowWrapper?: (element: BudgetElementDto, bucket: FolderBucket, row: ReactNode) => ReactNode
  /** wraps a category inside an unfolded envelope (its drag grip) */
  wrapChild?: (child: BudgetChildElementDto, parent: BudgetElementDto, node: ReactNode) => ReactNode
  /** wraps an unfolded envelope's category list (its drop zone) */
  wrapChildren?: (parent: BudgetElementDto, node: ReactNode) => ReactNode
  /** the ⋮ menu of a category inside an unfolded envelope */
  childMenu?: (child: BudgetChildElementDto, parent: BudgetElementDto) => MenuAction[] | undefined
  /** a folded envelope's own drop zone, inside its row */
  envelopeHeadDrop?: (element: BudgetElementDto) => ReactNode
  onSpentClick?: (target: BudgetTransactionsTarget) => void
  /** the row's ⋮ menu, shown on hover */
  rowMenu?: (element: BudgetElementDto) => MenuAction[] | undefined
}

interface BudgetTableProps extends ElementRowExtras {
  budget: BudgetDto
  buckets: BudgetBuckets
  /** a month that has not started: Spent reads as a dash and nothing turns red */
  future?: boolean
  /** the caller renders BudgetTotals itself (below the Savings block) */
  hideTotals?: boolean
  /** folder actions, right after the folder's name (edit mode) */
  renderFolderActions?: (bucket: FolderBucket, index: number, total: number) => ReactNode
  /** wraps folder/no-folder sections (dnd droppables in edit mode) */
  sectionWrapper?: (bucket: FolderBucket, sectionKey: string, node: ReactNode) => ReactNode
  /** the element being dragged: it renders collapsed */
  collapsedElementId?: string | null
  /** a drag is in progress: an empty No folder shows, as the drop target for
   *  taking an element out of its folder or envelope */
  showEmptyNoFolder?: boolean
  /** a FOLDER drag is in progress: sections render header-only */
  hideContents?: boolean
  /** folder drag handle, rendered before the folder name (edit mode) */
  renderFolderHandle?: (bucket: FolderBucket) => ReactNode
  /** a folder line's ⋮ menu (real folders and No folder) */
  folderMenu?: (bucket: FolderBucket) => MenuAction[] | undefined
  /** a reporting tag's ⋮ menu */
  labelMenu?: (label: LabelSpendDto) => MenuAction[] | undefined
  /** the Expenses line's ⋮ menu */
  sectionMenu?: MenuAction[]
}

const cellOpts = (currency: CurrencyDto | undefined): MoneyFormatOptions => ({
  showCurrency: false,
  useNativePrecision: false,
  maxPrecision: currency?.fractionDigits ?? 2,
})

/* an expense row's Available: a green pill while money is left, red once it runs out */
function AvailablePill({ available, currency }: { available: string; currency: CurrencyDto | undefined }) {
  return (
    <span
      data-testid="available-pill"
      data-pill=""
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-sm font-medium tabular-nums ${
        cmp(available, '0') >= 0 ? 'bg-income/10 text-income' : 'bg-expense/10 text-expense'
      }`}
    >
      {moneyFormat(available, currency, cellOpts(currency))}
    </span>
  )
}

/* An explanation available on demand. Kept out of any collapsible trigger:
   explaining a block must never fold it. */
export function InfoNote({ text, testId }: { text: string; testId: string }) {
  const { t } = useTranslation()
  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          className="shrink-0 text-muted-foreground hover:text-foreground"
          aria-label={t('common.button.info.label')}
          title={t('common.button.info.label')}
        >
          <Info className="size-3.5" />
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-72 text-xs text-muted-foreground" data-testid={testId}>
        {text}
      </PopoverContent>
    </Popover>
  )
}

/** what earlier months left, read-only, right before the budget: "530.00 + 700.00" */
function CarryLeadIn({ carry, text, testId }: { carry: string; text: string; testId: string }) {
  const { t } = useTranslation()
  return (
    <span
      data-testid={testId}
      title={t('budgets.page.budget.structure.carry_over_hint')}
      className={`shrink-0 text-[13px] ${cmp(carry, '0') < 0 ? 'text-expense' : 'text-muted-foreground'}`}
    >
      {text}
    </span>
  )
}

function ElementRow({
  element,
  bucket,
  budget,
  currencies,
  accessById,
  extras,
  actionsColumn = false,
  hideChildren = false,
  future = false,
}: {
  element: BudgetElementDto
  bucket: FolderBucket
  budget: BudgetDto
  currencies: CurrencyDto[]
  accessById: Map<string, UserDto>
  extras: ElementRowExtras
  /** the table renders an actions column (edit mode): rows without their own actions pad it */
  actionsColumn?: boolean
  hideChildren?: boolean
  future?: boolean
}) {
  const { t } = useTranslation()
  const level = useRowLevel()
  const unfolded = useBudgetPeriodStore((s) => !!s.unfoldedElements[element.id]) && !hideChildren
  const toggleElement = useBudgetPeriodStore((s) => s.toggleElement)

  const currencyId = element.currencyId ?? budget.meta.currencyId
  const currency = currencies.find((c) => c.id === currencyId)
  const available = displayAvailable(element)
  const carry = carryOver(element)
  // an envelope unfolds even while empty: its list is where a category is dropped
  const expandable = element.children.length > 0 || isEnvelopeType(element.type)
  const opts = cellOpts(currency)
  const showTransactionsTitle = t('budgets.page.budget.structure.element.action.show_transactions')
  const displayName = elementDisplayName(element.id, element.name, t)
  // categoryless spending can never be budgeted: those columns read as a dash
  const isUncategorized = element.id === UNCATEGORIZED_ID
  const carryText = isUncategorized || isZero(carry) ? null : `${moneyFormat(carry, currency, opts)} +`
  // red marks a problem only: this month spent more than its budget and what earlier
  // months left does not cover it
  const overspent = !isUncategorized && overBudget({ budgeted: element.budgeted, spent: element.spent, available }, future)
  const blank = isUncategorized || nothingToShow({ budgeted: element.budgeted, spent: element.spent, available })

  const spentCell = (target: BudgetTransactionsTarget, spent: string, danger: boolean) => {
    if (future) {
      return <Dash />
    }
    const color = danger ? 'text-expense' : 'text-muted-foreground'
    return extras.onSpentClick ? (
      <button
        type="button"
        title={showTransactionsTitle}
        aria-label={`transactions ${target.name}`}
        className={`tabular-nums underline-offset-2 hover:text-foreground hover:underline ${color}`}
        onClick={() => extras.onSpentClick!(target)}
      >
        {moneyFormat(spent, currency, opts)}
      </button>
    ) : (
      <span className={`tabular-nums ${color}`}>{moneyFormat(spent, currency, opts)}</span>
    )
  }

  const Chevron = unfolded ? ChevronDown : ChevronRight
  const name = (
    <>
      {expandable ? <Chevron className="size-3.5 shrink-0 text-muted-foreground" /> : <span className="w-3.5 shrink-0" />}
      {isUncategorized ? null : <EntityIcon name={element.icon} className="text-lg text-muted-foreground" />}
      <span className="truncate text-[15px]" title={displayName}>
        {displayName}
      </span>
      {currencyId !== budget.meta.currencyId && currency ? <CurrencyTag code={currency.code} /> : null}
      {isUncategorized ? <InfoNote text={t('budgets.page.budget.structure.uncategorized.info')} testId="budget-uncategorized-info-note" /> : null}
    </>
  )

  const budgetCell = (() => {
    const cell = (
      <span {...{ [COMMENT_ANCHOR_ATTR]: '' }} className={`group/cell relative ${FIRST_COL} text-[15px]`} data-testid="cell-budgeted">
        {carryText !== null ? <CarryLeadIn carry={carry} text={carryText} testId="cell-carry" /> : null}
        <span className="shrink-0">
          {isUncategorized ? (
            <Dash />
          ) : extras.onBudgetCellDetails ? (
            <button
              type="button"
              className="w-full text-right underline-offset-2 hover:underline"
              aria-label={`details ${displayName}`}
              onClick={() => extras.onBudgetCellDetails!(element)}
            >
              {moneyFormat(element.budgeted, currency, opts)}
            </button>
          ) : extras.renderBudgetCell ? (
            extras.renderBudgetCell(element)
          ) : extras.onBudgetCellComments ? (
            <button
              type="button"
              className="w-full text-right underline-offset-2 hover:underline"
              aria-label={`comments ${displayName}`}
              onClick={(e) => extras.onBudgetCellComments!(element, commentAnchorOf(e.currentTarget))}
            >
              {moneyFormat(element.budgeted, currency, opts)}
            </button>
          ) : (
            moneyFormat(element.budgeted, currency, opts)
          )}
        </span>
        {extras.renderBudgetCellMarker?.(element)}
      </span>
    )
    return !isUncategorized && extras.wrapBudgetCell ? extras.wrapBudgetCell(element, cell) : cell
  })()

  const childList = (
    <div className="pb-1" data-testid={`children-${element.id}`}>
      {element.children.length === 0 ? (
        <p className={`${CHILD_INDENT[level]} px-2 py-1 text-xs text-muted-foreground`}>{t('budgets.page.budget.structure.empty_envelope.note')}</p>
      ) : null}
      {element.children.map((child) => {
        const owner = accessById.size > 1 && child.ownerUserId ? accessById.get(child.ownerUserId) : undefined
        const childDisplayName = elementDisplayName(child.id, child.name, t)
        const line = (
          <div
            key={child.id}
            className={`group ${LINE} ${CHILD_INDENT[level]} min-h-8 rounded-md py-1 text-sm text-muted-foreground hover:bg-accent/50`}
            data-testid={`child-${child.id}`}
          >
            <span className={NAME_COL}>
              <EntityIcon name={child.icon} className="text-lg" />
              <span className="truncate" title={childDisplayName}>
                {childDisplayName}
              </span>
            </span>
            <RowMenu name={childDisplayName} actions={isEnvelopeType(element.type) ? extras.childMenu?.(child, element) : undefined} />
            {/* owner sits in the budget column, flush under the amounts; row hover only (multi-user budgets) */}
            <span className={`${FIRST_COL} truncate text-xs text-muted-foreground/60 opacity-0 group-hover:opacity-100`}>{owner?.name}</span>
            <span data-testid="child-spent" className={SECOND_COL}>
              {spentCell(
                { id: child.id, type: child.type, name: childDisplayName, icon: child.icon, currencyId: element.currencyId, parent: { id: element.id, type: element.type } },
                child.spent,
                false,
              )}
            </span>
            <span className={THIRD_COL} />
            {actionsColumn ? <ActionsSpacer /> : null}
          </div>
        )
        return extras.wrapChild ? <div key={child.id}>{extras.wrapChild(child, element, line)}</div> : line
      })}
    </div>
  )

  const row = (
    <div className="flex flex-col" data-testid={`element-${element.id}`}>
      <div className={`${LINE} ${ROW_INDENT[level]} relative min-h-10 rounded-md py-1.5 hover:bg-accent/50`}>
        {expandable ? (
          <button
            type="button"
            className={`${NAME_COL} text-left`}
            aria-expanded={unfolded}
            title={t(unfolded ? 'common.button.collapse.label' : 'common.button.expand.label')}
            onClick={() => toggleElement(element.id)}
          >
            {name}
          </button>
        ) : (
          <span className={NAME_COL}>{name}</span>
        )}
        <RowMenu name={displayName} actions={extras.rowMenu?.(element)} />
        {budgetCell}
        <span data-testid="cell-spent" className={`${SECOND_COL} text-[15px]`}>
          {spentCell({ id: element.id, type: element.type, name: displayName, icon: element.icon, currencyId: element.currencyId }, element.spent, overspent)}
        </span>
        <span data-testid="cell-available" className={`${THIRD_COL} text-[15px] ${blank ? 'text-muted-foreground' : ''}`}>
          {blank ? <Dash /> : <AvailablePill available={available} currency={currency} />}
        </span>
        {extras.renderActions ? extras.renderActions(element, bucket) : actionsColumn ? <ActionsSpacer /> : null}
        {extras.envelopeHeadDrop && isEnvelopeType(element.type) && !unfolded ? extras.envelopeHeadDrop(element) : null}
      </div>
      {expandable && unfolded ? (extras.wrapChildren ? extras.wrapChildren(element, childList) : childList) : null}
    </div>
  )

  return extras.renderRowWrapper ? <>{extras.renderRowWrapper(element, bucket, row)}</> : row
}

/** one reporting tag: the row columns, with the amount under Spent — a label has
 *  only one amount, so Budget and Available read as dashes */
function LabelRow({
  label,
  currency,
  opts,
  future,
  onLabelClick,
  menu,
}: {
  label: LabelSpendDto
  currency: CurrencyDto | undefined
  opts: MoneyFormatOptions
  future: boolean
  onLabelClick?: (target: BudgetTransactionsTarget) => void
  menu?: MenuAction[]
}) {
  const { t } = useTranslation()
  const level = useRowLevel()
  const unfolded = useBudgetPeriodStore((s) => !!s.unfoldedElements[label.id])
  const toggleElement = useBudgetPeriodStore((s) => s.toggleElement)

  const children = label.children ?? []
  const expandable = children.length > 0
  const showTransactionsTitle = t('budgets.page.budget.structure.element.action.show_transactions')
  const Chevron = unfolded ? ChevronDown : ChevronRight

  const spentCell = (target: BudgetTransactionsTarget, spent: string) =>
    future ? (
      <Dash />
    ) : onLabelClick ? (
      <button
        type="button"
        title={showTransactionsTitle}
        aria-label={`transactions ${target.name}`}
        className="tabular-nums text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
        onClick={() => onLabelClick(target)}
      >
        {moneyFormat(spent, currency, opts)}
      </button>
    ) : (
      <span className="tabular-nums text-muted-foreground">{moneyFormat(spent, currency, opts)}</span>
    )

  const name = (
    <>
      {expandable ? <Chevron className="size-3.5 shrink-0 text-muted-foreground" /> : <span className="w-3.5 shrink-0" />}
      <EntityIcon name={label.icon} className="text-lg text-muted-foreground" />
      <span className={`truncate text-[15px] ${label.isArchived === 1 ? 'text-muted-foreground' : ''}`} title={label.name}>
        {label.name}
      </span>
    </>
  )

  return (
    <li className="flex flex-col" data-testid={`budget-label-${label.id}`}>
      <div className={`${LINE} ${ROW_INDENT[level]} relative min-h-10 rounded-md py-1.5 hover:bg-accent/50`}>
        {expandable ? (
          <button
            type="button"
            className={`${NAME_COL} text-left`}
            aria-expanded={unfolded}
            title={t(unfolded ? 'common.button.collapse.label' : 'common.button.expand.label')}
            onClick={() => toggleElement(label.id)}
          >
            {name}
          </button>
        ) : (
          <span className={NAME_COL}>{name}</span>
        )}
        <RowMenu name={label.name} actions={menu} />
        <span className={`${FIRST_COL} text-[15px]`}>
          <Dash />
        </span>
        <span className={`${SECOND_COL} text-[15px]`}>
          {spentCell({ id: label.id, type: 'label', name: label.name, icon: label.icon, currencyId: null }, label.spent)}
        </span>
        <span className={`${THIRD_COL} text-[15px]`}>
          <Dash />
        </span>
      </div>
      {expandable && unfolded ? (
        <ul className="pb-1">
          {children.map((child) => {
            const childDisplayName = elementDisplayName(child.id, child.name, t)
            return (
              <li
                key={child.id}
                className={`${LINE} ${CHILD_INDENT[level]} min-h-8 rounded-md py-1 text-sm text-muted-foreground hover:bg-accent/50`}
                data-testid={`label-child-${child.id}`}
              >
                <span className={NAME_COL}>
                  <EntityIcon name={child.icon} className="text-lg" />
                  <span className="truncate" title={childDisplayName}>
                    {childDisplayName}
                  </span>
                </span>
                <span className={FIRST_COL} />
                <span className={SECOND_COL}>
                  {spentCell(
                    {
                      id: child.id,
                      type: child.type,
                      name: childDisplayName,
                      icon: child.icon,
                      currencyId: null,
                      // the child is this category's slice of THIS tag, not the
                      // whole category: the drill-down must filter by both
                      parent: { id: label.id, type: 'label' },
                    },
                    child.spent,
                  )}
                </span>
                <span className={THIRD_COL} />
              </li>
            )
          })}
        </ul>
      ) : null}
    </li>
  )
}

function ReportingTagsFolder({
  labels,
  currency,
  future,
  actionsColumn,
  onLabelClick,
  labelMenu,
}: {
  labels: LabelSpendDto[]
  currency: CurrencyDto | undefined
  future: boolean
  actionsColumn: boolean
  onLabelClick?: (target: BudgetTransactionsTarget) => void
  labelMenu?: (label: LabelSpendDto) => MenuAction[] | undefined
}) {
  const { t } = useTranslation()
  const open = useBudgetPeriodStore((s) => !!s.unfoldedElements[REPORTING_TAGS_FOLD_ID])
  const toggleElement = useBudgetPeriodStore((s) => s.toggleElement)
  const opts = cellOpts(currency)
  const Chevron = open ? ChevronDown : ChevronRight

  return (
    <section data-testid="budget-labels-section">
      <div
        className={`${LINE} ${FOLD_LINE} ${FOLDER_INDENT} min-h-9 text-sm text-muted-foreground`}
        onClick={foldOnLineClick(() => toggleElement(REPORTING_TAGS_FOLD_ID))}
      >
        <button
          type="button"
          data-fold=""
          className="flex min-w-0 items-center gap-1.5 py-1 text-left"
          aria-expanded={open}
          title={t(open ? 'common.button.collapse.label' : 'common.button.expand.label')}
        >
          <Chevron className="size-3.5 shrink-0" />
          <span className="min-w-0 truncate" data-testid="budget-labels-heading">
            {t('budgets.page.budget.structure.labels.heading')}
          </span>
        </button>
        <InfoNote text={t('budgets.page.budget.structure.labels.info')} testId="budget-labels-info-note" />
        <span className="flex-1" />
        {actionsColumn ? <ActionsSpacer /> : null}
      </div>
      {open ? (
        <ul>
          {labels.map((label) => (
            <LabelRow key={label.id} label={label} currency={currency} opts={opts} future={future} onLabelClick={onLabelClick} menu={labelMenu?.(label)} />
          ))}
        </ul>
      ) : null}
    </section>
  )
}

/** a folder's sums, in the row columns: muted, and red only where the folder as a
 *  whole overspent */
function folderSums(stats: BucketStats, currency: CurrencyDto | undefined, future: boolean): [ReactNode, ReactNode, ReactNode] {
  const opts = cellOpts(currency)
  const danger = overBudget(stats, future)
  return [
    moneyFormat(stats.budgeted, currency, opts),
    future ? <Dash key="dash-452-1" /> : <span key="spent" className={danger ? 'text-expense' : ''}>{moneyFormat(stats.spent, currency, opts)}</span>,
    nothingToShow(stats) ? <Dash key="dash-453-1" /> : <span key="available" className={danger ? 'text-expense' : ''}>{moneyFormat(stats.available, currency, opts)}</span>,
  ]
}

export function BudgetTable({
  budget,
  buckets,
  future = false,
  renderFolderActions,
  renderFolderHandle,
  sectionWrapper,
  collapsedElementId,
  showEmptyNoFolder = false,
  hideContents,
  hideTotals,
  folderMenu,
  labelMenu,
  sectionMenu,
  ...extras
}: BudgetTableProps) {
  const { t } = useTranslation()
  const { data: currencies = [] } = useCurrencies()
  const budgetCurrency = currencies.find((c) => c.id === budget.meta.currencyId)
  const totals = budgetTotals(buckets)
  const actionsColumn = !!extras.renderActions
  const accessById = new Map(budget.meta.access.map((a) => [a.user.id, a.user]))
  const labels = budget.structure.labels ?? []
  const folded = useBudgetPeriodStore((s) => !!s.planFolds.expense)
  const planFolds = useBudgetPeriodStore((s) => s.planFolds)
  const togglePlanFold = useBudgetPeriodStore((s) => s.togglePlanFold)

  const realFolders = buckets.withFolder
  // fold keys: a folder's own id (the Plan grid's), '__no_folder__', and 'archived'
  // (the Plan grid's Archived band)
  const sections: { key: string; foldKey: string; name: string; bucket: FolderBucket; folderIndex: number | null }[] = [
    ...realFolders.map((bucket, index) => ({ key: bucket.folder!.id, foldKey: bucket.folder!.id, name: bucket.folder!.name, bucket, folderIndex: index })),
    { key: '__no_folder__', foldKey: '__no_folder__', name: t('budgets.page.plan.menu.no_folder'), bucket: buckets.withoutFolder, folderIndex: null },
    { key: '__uncategorized__', foldKey: '__uncategorized__', name: t('common.uncategorized'), bucket: buckets.uncategorized, folderIndex: null },
    { key: '__archive__', foldKey: 'archived', name: t('budgets.page.budget.structure.in_archive'), bucket: buckets.archive, folderIndex: null },
  ]
  const opts = cellOpts(budgetCurrency)

  const rowsOf = (bucket: FolderBucket, rowExtras: ElementRowExtras) =>
    bucket.elements.map((element) => (
      <ElementRow
        key={element.id}
        element={element}
        bucket={bucket}
        budget={budget}
        currencies={currencies}
        accessById={accessById}
        extras={rowExtras}
        actionsColumn={actionsColumn}
        hideChildren={collapsedElementId === element.id}
        future={future}
      />
    ))

  return (
    <div className="flex flex-col border-t pt-1" data-testid="budget-table">
      <MonthSectionHeader
        foldKey="expense"
        testId="column-headers"
        label={t('budgets.page.plan.section.expenses')}
        headings={[t('budgets.page.budget.structure.tab.budgeted'), t('budgets.page.budget.structure.tab.spent'), t('budgets.page.budget.structure.tab.available')]}
        sums={[
          moneyFormat(totals.budgeted, budgetCurrency, opts),
          future ? <Dash key="dash-516-1" /> : moneyFormat(totals.spent, budgetCurrency, opts),
          nothingToShow(totals) ? <Dash key="dash-517-1" /> : moneyFormat(totals.available, budgetCurrency, opts),
        ]}
        actionsColumn={actionsColumn}
        menu={sectionMenu}
      />

      {folded
        ? null
        : sections.flatMap((section) => {
            // archive and uncategorized are read-only: no drag handle, no folder
            // actions, never a drop container
            const isReadOnlySection = section.key === '__archive__' || section.key === '__uncategorized__'
            // Uncategorized is a single fixed row, not a group: it renders flat,
            // with no line of its own, so the label appears once
            if (section.key === '__uncategorized__') {
              if (section.bucket.elements.length === 0) {
                return []
              }
              return [
                <section key={section.key} data-testid={`budget-folder-${section.name}`}>
                  <RowLevelContext.Provider value="top">
                    {rowsOf(section.bucket, { onSpentClick: extras.onSpentClick, rowMenu: extras.rowMenu })}
                  </RowLevelContext.Provider>
                </section>,
              ]
            }
            if (section.bucket.elements.length === 0 && section.folderIndex === null) {
              // both read-only sections hide when they have nothing to show; the
              // empty No folder survives only as a drop target, while a drag is
              // in progress (or in the folder-actions edit mode)
              if (isReadOnlySection || realFolders.length === 0 || !(renderFolderActions || showEmptyNoFolder)) {
                return []
              }
            }
            // without real folders the folder-less rows need no line naming them
            const named = section.key !== '__no_folder__' || realFolders.length > 0 || !!renderFolderActions
            const sectionFolded = named && !!planFolds[section.foldKey]
            const rowExtras: ElementRowExtras = isReadOnlySection
              ? {
                  onSpentClick: extras.onSpentClick,
                  // read affordances only: an individually-archived element keeps
                  // its existing thread reachable (marker) and can still gain new
                  // comments (the thread) — only the WRITE affordances (limit
                  // editing, drag, folder actions) are read-only here. This branch
                  // is reached only by the Archive section (Uncategorized returns
                  // earlier, above, with its own fixed extras)
                  renderBudgetCellMarker: extras.renderBudgetCellMarker,
                  onBudgetCellComments: extras.onBudgetCellComments,
                  onBudgetCellDetails: extras.onBudgetCellDetails,
                  wrapBudgetCell: extras.wrapBudgetCell,
                  rowMenu: extras.rowMenu,
                }
              : extras
            const sectionNode = (
              <section key={section.key} className="pt-1" data-testid={`budget-folder-${section.name}`}>
                {named ? (
                  <FolderLine
                    name={section.name}
                    folded={sectionFolded}
                    onToggle={() => togglePlanFold(section.foldKey)}
                    sums={section.bucket.elements.length > 0 ? folderSums(section.bucket.stats, budgetCurrency, future) : null}
                    handle={!isReadOnlySection ? renderFolderHandle?.(section.bucket) : null}
                    actions={!isReadOnlySection ? renderFolderActions?.(section.bucket, section.folderIndex ?? -1, realFolders.length) : null}
                    actionsColumn={actionsColumn}
                    menu={!isReadOnlySection ? folderMenu?.(section.bucket) : undefined}
                  />
                ) : null}
                {hideContents || sectionFolded ? null : section.bucket.elements.length === 0 ? (
                  <p className={`${ROW_INDENT['in-folder']} px-2 py-1 text-xs text-muted-foreground`}>{t('budgets.page.budget.structure.empty_folder.note')}</p>
                ) : named ? (
                  rowsOf(section.bucket, rowExtras)
                ) : (
                  // no folder line above: the rows sit at its step
                  <RowLevelContext.Provider value="top">{rowsOf(section.bucket, rowExtras)}</RowLevelContext.Provider>
                )}
              </section>
            )
            return [
              !isReadOnlySection && sectionWrapper ? (
                <div key={section.key}>{sectionWrapper(section.bucket, section.key, sectionNode)}</div>
              ) : (
                sectionNode
              ),
            ]
          })}

      {/* an ephemeral folder, last: none of the edit-mode props (folder
          actions, drag handles, section/row wrappers) reach it, so it can
          never be renamed, moved, deleted, or become a drop target */}
      {!folded && labels.length > 0 ? (
        <ReportingTagsFolder
          labels={labels}
          currency={budgetCurrency}
          future={future}
          actionsColumn={actionsColumn}
          onLabelClick={extras.onSpentClick}
          labelMenu={labelMenu}
        />
      ) : null}

      {hideTotals ? null : <BudgetTotals budget={budget} totals={totals} actionsColumn={actionsColumn} future={future} />}
    </div>
  )
}

/** The Total row (desktop) and its phone card. The budget page renders it itself,
 *  below the Savings block, with the savings rows added in. */
export function BudgetTotals({
  budget,
  totals,
  actionsColumn,
  future = false,
}: {
  budget: BudgetDto
  totals: BucketStats
  actionsColumn: boolean
  future?: boolean
}) {
  const { t } = useTranslation()
  const { data: currencies = [] } = useCurrencies()
  const budgetCurrency = currencies.find((c) => c.id === budget.meta.currencyId)
  const opts = cellOpts(budgetCurrency)
  const danger = overBudget(totals, future)
  return (
    <>
      <div className={`hidden ${LINE} min-h-10 border-t border-foreground/20 py-1.5 text-[15px] sm:flex`} data-testid="budget-totals">
        <span className={NAME_COL}>
          <span className="truncate">{t('budgets.page.budget.structure.total.name')}</span>
        </span>
        <span className={FIRST_COL}>
          {!isZero(totals.carry) ? <CarryLeadIn carry={totals.carry} text={`${moneyFormat(totals.carry, budgetCurrency, opts)} +`} testId="totals-carry" /> : null}
          <span className="shrink-0">{moneyFormat(totals.budgeted, budgetCurrency, opts)}</span>
        </span>
        <span className={`${SECOND_COL} ${danger ? 'text-expense' : 'text-muted-foreground'}`}>
          {future ? <Dash /> : moneyFormat(totals.spent, budgetCurrency, opts)}
        </span>
        <span className={`${THIRD_COL} ${danger ? 'text-expense' : ''}`} data-testid="totals-available">
          {nothingToShow(totals) ? <Dash /> : moneyFormat(totals.available, budgetCurrency, opts)}
        </span>
        {actionsColumn ? <ActionsSpacer /> : null}
      </div>

      {/* the phone table hides the budget column, so the totals unfold into
          labeled lines; the margin keeps the card off the very screen edge */}
      <div
        className="mb-[max(env(safe-area-inset-bottom),0.75rem)] flex flex-col gap-2 rounded-md border px-3 py-2.5 sm:hidden"
        data-testid="budget-totals-mobile"
      >
        <span className="text-[15px]">{t('budgets.page.budget.structure.total.name')}</span>
        <span className="flex items-baseline justify-between">
          <span className="text-[13px] text-muted-foreground">{t('budgets.page.budget.structure.tab.budgeted')}</span>
          <span className="text-[15px] tabular-nums">{moneyFormat(totals.budgeted, budgetCurrency, opts)}</span>
        </span>
        <span className="flex items-baseline justify-between">
          <span className="text-[13px] text-muted-foreground">{t('budgets.page.budget.structure.tab.spent')}</span>
          <span className="text-[15px] tabular-nums text-muted-foreground">{moneyFormat(totals.spent, budgetCurrency, opts)}</span>
        </span>
        <span className="flex items-baseline justify-between">
          <span className="text-[13px] text-muted-foreground">{t('budgets.page.budget.structure.tab.available')}</span>
          <span className={`text-[15px] tabular-nums ${danger ? 'text-expense' : ''}`}>{moneyFormat(totals.available, budgetCurrency, opts)}</span>
        </span>
      </div>
    </>
  )
}
