import type { ReactNode } from 'react'
import { ChevronDown, ChevronRight, Info } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { EntityIcon } from '@/components/EntityIcon'
import { cmp } from '@/lib/decimal'
import { moneyFormat } from '@/lib/money'
import type { MoneyFormatOptions } from '@/lib/money'
import type { BudgetDto, BudgetElementDto, LabelSpendDto } from '@/api/dto/budget'
import { UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { UserDto } from '@/api/dto/user'
import { useCurrencies } from '@/features/currencies/queries'
import type { BudgetBuckets, BucketStats, FolderBucket } from './budgetMath'
import { budgetTotals, displayAvailable, elementDisplayName } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'
import type { BudgetTransactionsTarget } from './BudgetTransactionsDialog'

export interface ElementRowExtras {
  /** the budget cell contents (set-limit editor) — defaults to a plain value */
  renderBudgetCell?: (element: BudgetElementDto) => ReactNode
  /** trailing actions (edit-mode menus, drag handle) */
  renderActions?: (element: BudgetElementDto, bucket: FolderBucket) => ReactNode
  renderRowWrapper?: (element: BudgetElementDto, bucket: FolderBucket, row: ReactNode) => ReactNode
  onSpentClick?: (target: BudgetTransactionsTarget) => void
  /** compact screens hide the budget column — tapping Available opens the set-limit dialog instead */
  onAvailableClick?: (element: BudgetElementDto) => void
}

interface BudgetTableProps extends ElementRowExtras {
  budget: BudgetDto
  buckets: BudgetBuckets
  renderFolderActions?: (bucket: FolderBucket, index: number, total: number) => ReactNode
  /** wraps folder/no-folder sections (dnd droppables in edit mode) */
  sectionWrapper?: (bucket: FolderBucket, sectionKey: string, node: ReactNode) => ReactNode
  /** an element drag is in progress: unfolded rows render collapsed */
  hideChildren?: boolean
  /** a FOLDER drag is in progress: sections render header-only */
  hideContents?: boolean
  /** folder drag handle, rendered before the folder name (edit mode) */
  renderFolderHandle?: (bucket: FolderBucket) => ReactNode
}

// em dash: a column that carries no value at all, as opposed to a zero
const EMPTY_CELL = '—'

const cellOpts = (currency: CurrencyDto | undefined): MoneyFormatOptions => ({
  showCurrency: false,
  useNativePrecision: false,
  maxPrecision: currency?.fractionDigits ?? 2,
})

function AvailablePill({ available, currency, testId }: { available: string; currency: CurrencyDto | undefined; testId?: string }) {
  return (
    <span
      data-testid={testId}
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium tabular-nums ${
        cmp(available, '0') >= 0 ? 'bg-income/10 text-income' : 'bg-expense/10 text-expense'
      }`}
    >
      {moneyFormat(available, currency, cellOpts(currency))}
    </span>
  )
}

function StatCells({ stats, currency, hideSymbol = false }: { stats: BucketStats; currency: CurrencyDto | undefined; hideSymbol?: boolean }) {
  const opts = cellOpts(currency)
  const available = stats.available
  return (
    <span className="flex items-center gap-2 text-xs text-muted-foreground" data-testid="stat-line">
      <span className="hidden w-24 text-right tabular-nums sm:block">{moneyFormat(stats.budgeted, currency, opts)}</span>
      <span className="w-20 text-center tabular-nums sm:w-24">{moneyFormat(stats.spent, currency, opts)}</span>
      <span className={`w-20 text-center tabular-nums sm:w-24 ${cmp(available, '0') >= 0 ? 'text-income' : 'text-expense'}`}>
        {moneyFormat(available, currency, opts)}
      </span>
      {hideSymbol ? null : <span className="hidden w-6 text-center sm:block">{currency?.symbol}</span>}
    </span>
  )
}


/* An explanation available on demand. Kept out of any collapsible trigger:
   explaining a block must never fold it. */
function InfoNote({ text, testId }: { text: string; testId: string }) {
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

/* edit mode appends a w-8 actions button to element rows; every row without
   one must pad the slot or its amount columns drift out of alignment */
function ActionsSpacer() {
  return <span data-testid="actions-spacer" className="w-8 shrink-0" />
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
}) {
  const { t } = useTranslation()
  const unfolded = useBudgetPeriodStore((s) => !!s.unfoldedElements[element.id]) && !hideChildren
  const toggleElement = useBudgetPeriodStore((s) => s.toggleElement)

  const currencyId = element.currencyId ?? budget.meta.currencyId
  const currency = currencies.find((c) => c.id === currencyId)
  const available = displayAvailable(element)
  const expandable = element.children.length > 0
  const opts = cellOpts(currency)
  const showTransactionsTitle = t('budgets.page.budget.structure.element.action.show_transactions')
  const displayName = elementDisplayName(element.id, element.name, t)
  // categoryless spending can never be budgeted: those columns read as a dash
  const isUncategorized = element.id === UNCATEGORIZED_ID

  const spentCell = (target: BudgetTransactionsTarget, spent: string) =>
    extras.onSpentClick ? (
      <button
        type="button"
        title={showTransactionsTitle}
        aria-label={`transactions ${target.name}`}
        className="w-20 text-center text-[15px] tabular-nums text-muted-foreground underline-offset-2 hover:text-foreground hover:underline sm:w-24"
        onClick={() => extras.onSpentClick!(target)}
      >
        {moneyFormat(spent, currency, opts)}
      </button>
    ) : (
      <span className="w-20 text-center text-[15px] tabular-nums text-muted-foreground sm:w-24">
        {moneyFormat(spent, currency, opts)}
      </span>
    )

  // mobile has no room for a chevron column: the chevron replaces the entity
  // icon on expandable rows, childless rows drop the alignment spacer
  const Chevron = unfolded ? ChevronDown : ChevronRight
  const name = (
    <>
      {expandable ? (
        <Chevron className="hidden size-3.5 shrink-0 text-muted-foreground sm:block" />
      ) : (
        <span className="hidden w-3.5 shrink-0 sm:block" />
      )}
      {expandable ? (
        <>
          <Chevron className="size-4.5 shrink-0 text-muted-foreground sm:hidden" />
          {/* wrapper span: .material-icon's own display beats the `hidden` utility */}
          <span className="hidden sm:block">
            <EntityIcon name={element.icon} className="text-lg text-muted-foreground" />
          </span>
        </>
      ) : isUncategorized ? (
        // mobile keeps the icon — it is the row's only visual anchor there;
        // on desktop the label alone carries the (single, fixed) row
        <span className="sm:hidden">
          <EntityIcon name={element.icon} className="text-lg text-muted-foreground" />
        </span>
      ) : (
        <EntityIcon name={element.icon} className="text-lg text-muted-foreground" />
      )}
      <span className="truncate text-[15px]" title={displayName}>
        {displayName}
      </span>
      {isUncategorized ? <InfoNote text={t('budgets.page.budget.structure.uncategorized.info')} testId="budget-uncategorized-info-note" /> : null}
    </>
  )

  const row = (
    <div className="flex flex-col" data-testid={`element-${element.id}`}>
      <div className="flex items-center gap-1.5 rounded-md px-1.5 py-2.5 hover:bg-accent/50 sm:gap-2 sm:px-2">
        {expandable ? (
          <button
            type="button"
            className="flex min-w-0 flex-1 items-center gap-2 text-left"
            aria-expanded={unfolded}
            title={t(unfolded ? 'common.button.collapse.label' : 'common.button.expand.label')}
            onClick={() => toggleElement(element.id)}
          >
            {name}
          </button>
        ) : (
          <span className="flex min-w-0 flex-1 items-center gap-2">{name}</span>
        )}
        <span className="hidden w-24 text-right text-[15px] tabular-nums sm:block" data-testid="cell-budgeted">
          {isUncategorized ? (
            EMPTY_CELL
          ) : extras.renderBudgetCell ? (
            extras.renderBudgetCell(element)
          ) : (
            moneyFormat(element.budgeted, currency, opts)
          )}
        </span>
        <span data-testid="cell-spent" className="flex justify-end">
          {spentCell(
            { id: element.id, type: element.type, name: displayName, icon: element.icon, currencyId: element.currencyId },
            element.spent,
          )}
        </span>
        <span className="flex w-20 justify-center sm:w-24">
          {isUncategorized ? (
            <span data-testid="cell-available" className="text-[15px] tabular-nums text-muted-foreground">
              {EMPTY_CELL}
            </span>
          ) : extras.onAvailableClick ? (
            <button
              type="button"
              title={t('budgets.modal.set_limit_form.header')}
              aria-label={`limit ${displayName}`}
              onClick={() => extras.onAvailableClick!(element)}
            >
              <AvailablePill available={available} currency={currency} testId="cell-available" />
            </button>
          ) : (
            <AvailablePill available={available} currency={currency} testId="cell-available" />
          )}
        </span>
        <span className="hidden w-6 text-center text-xs text-muted-foreground sm:block">{currency?.symbol}</span>
        {extras.renderActions ? extras.renderActions(element, bucket) : actionsColumn ? <ActionsSpacer /> : null}
      </div>
      {expandable && unfolded ? (
        <ul className="pb-1">
          {element.children.map((child) => {
            const owner = accessById.size > 1 && child.ownerUserId ? accessById.get(child.ownerUserId) : undefined
            const childDisplayName = elementDisplayName(child.id, child.name, t)
            return (
              <li
                key={child.id}
                className="group flex items-center gap-1.5 rounded-md py-1.5 pl-8 pr-1.5 text-sm text-muted-foreground hover:bg-accent/50 sm:gap-2 sm:pl-12 sm:pr-2"
                data-testid={`child-${child.id}`}
              >
                <EntityIcon name={child.icon} className="text-lg" />
                <span className="min-w-0 flex-1 truncate" title={childDisplayName}>
                  {childDisplayName}
                </span>
                {/* owner sits in the budget column slot, flush under the amounts; row hover only (multi-user budgets) */}
                <span className="hidden w-24 truncate text-right text-xs text-muted-foreground/60 opacity-0 group-hover:opacity-100 sm:block">
                  {owner?.name}
                </span>
                <span data-testid="child-spent" className="flex justify-end">
                  {spentCell({ id: child.id, type: child.type, name: childDisplayName, icon: child.icon, currencyId: element.currencyId, parent: { id: element.id, type: element.type } }, child.spent)}
                </span>
                <span className="w-20 sm:w-24" />
                <span className="hidden w-6 sm:block" />
                {actionsColumn ? <ActionsSpacer /> : null}
              </li>
            )
          })}
        </ul>
      ) : null}
    </div>
  )

  return extras.renderRowWrapper ? <>{extras.renderRowWrapper(element, bucket, row)}</> : row
}

/** the reporting-tags folder exists only in rendering: it has no folder row
 *  behind it, so it is keyed by a reserved literal that no real element id
 *  (a UUID) can collide with, and both fold levels persist like real ones */
const REPORTING_TAGS_FOLD_ID = '__reporting_tags__'

/** one reporting tag: the same [name flex-1][budgeted w-24][spent w-20/24][available w-20/24][symbol w-6]
 *  geometry as ElementRow, so the amount lands under the Spent header and gets
 *  a currency symbol like every neighbouring row -- a label has only one
 *  amount, so budgeted/available render as the same empty-cell dash Uncategorized uses */
function LabelRow({
  label,
  currency,
  opts,
  onLabelClick,
}: {
  label: LabelSpendDto
  currency: CurrencyDto | undefined
  opts: MoneyFormatOptions
  onLabelClick?: (target: BudgetTransactionsTarget) => void
}) {
  const { t } = useTranslation()
  const unfolded = useBudgetPeriodStore((s) => !!s.unfoldedElements[label.id])
  const toggleElement = useBudgetPeriodStore((s) => s.toggleElement)

  const children = label.children ?? []
  const expandable = children.length > 0
  const showTransactionsTitle = t('budgets.page.budget.structure.element.action.show_transactions')
  const Chevron = unfolded ? ChevronDown : ChevronRight

  const spentCell = (target: BudgetTransactionsTarget, spent: string) =>
    onLabelClick ? (
      <button
        type="button"
        title={showTransactionsTitle}
        aria-label={`transactions ${target.name}`}
        className="w-20 text-center text-[15px] tabular-nums text-muted-foreground underline-offset-2 hover:text-foreground hover:underline sm:w-24"
        onClick={() => onLabelClick(target)}
      >
        {moneyFormat(spent, currency, opts)}
      </button>
    ) : (
      <span className="w-20 text-center text-[15px] tabular-nums text-muted-foreground sm:w-24">
        {moneyFormat(spent, currency, opts)}
      </span>
    )

  // mirrors ElementRow: on mobile the chevron replaces the entity icon, since
  // there is no room for a separate chevron column
  const name = (
    <>
      {expandable ? (
        <Chevron className="hidden size-3.5 shrink-0 text-muted-foreground sm:block" />
      ) : (
        <span className="hidden w-3.5 shrink-0 sm:block" />
      )}
      {expandable ? (
        <>
          <Chevron className="size-4.5 shrink-0 text-muted-foreground sm:hidden" />
          {/* wrapper span: .material-icon's own display beats the `hidden` utility */}
          <span className="hidden sm:block">
            <EntityIcon name={label.icon} className="text-lg text-muted-foreground" />
          </span>
        </>
      ) : (
        <EntityIcon name={label.icon} className="text-lg text-muted-foreground" />
      )}
      <span className={`truncate text-[15px] ${label.isArchived === 1 ? 'text-muted-foreground' : ''}`} title={label.name}>
        {label.name}
      </span>
    </>
  )

  return (
    <li className="flex flex-col" data-testid={`budget-label-${label.id}`}>
      <div className="flex items-center gap-1.5 rounded-md px-1.5 py-2.5 hover:bg-accent/50 sm:gap-2 sm:px-2">
        {expandable ? (
          <button
            type="button"
            className="flex min-w-0 flex-1 items-center gap-2 text-left"
            aria-expanded={unfolded}
            title={t(unfolded ? 'common.button.collapse.label' : 'common.button.expand.label')}
            onClick={() => toggleElement(label.id)}
          >
            {name}
          </button>
        ) : (
          <span className="flex min-w-0 flex-1 items-center gap-2">{name}</span>
        )}
        <span className="hidden w-24 text-right text-[15px] tabular-nums sm:block">{EMPTY_CELL}</span>
        <span className="flex justify-end">
          {spentCell({ id: label.id, type: 'label', name: label.name, icon: label.icon, currencyId: null }, label.spent)}
        </span>
        <span className="flex w-20 justify-center text-[15px] tabular-nums text-muted-foreground sm:w-24">{EMPTY_CELL}</span>
        <span className="hidden w-6 text-center text-xs text-muted-foreground sm:block">{currency?.symbol}</span>
      </div>
      {expandable && unfolded ? (
        <ul className="pb-1">
          {children.map((child) => {
            const childDisplayName = elementDisplayName(child.id, child.name, t)
            return (
              <li
                key={child.id}
                className="group flex items-center gap-1.5 rounded-md py-1.5 pl-8 pr-1.5 text-sm text-muted-foreground hover:bg-accent/50 sm:gap-2 sm:pl-12 sm:pr-2"
                data-testid={`label-child-${child.id}`}
              >
                <EntityIcon name={child.icon} className="text-lg" />
                <span className="min-w-0 flex-1 truncate" title={childDisplayName}>
                  {childDisplayName}
                </span>
                <span className="hidden w-24 sm:block" />
                <span className="flex justify-end">
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
                <span className="w-20 sm:w-24" />
                <span className="hidden w-6 sm:block" />
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
  onLabelClick,
}: {
  labels: LabelSpendDto[]
  currency: CurrencyDto | undefined
  onLabelClick?: (target: BudgetTransactionsTarget) => void
}) {
  const { t } = useTranslation()
  const open = useBudgetPeriodStore((s) => !!s.unfoldedElements[REPORTING_TAGS_FOLD_ID])
  const toggleElement = useBudgetPeriodStore((s) => s.toggleElement)
  const opts = cellOpts(currency)

  return (
    <Collapsible open={open} onOpenChange={() => toggleElement(REPORTING_TAGS_FOLD_ID)}>
      <section className="rounded-md border p-1.5 sm:p-2" data-testid="budget-labels-section">
        <div className="flex items-center gap-1.5 px-1.5 pb-1 sm:gap-2 sm:px-2">
          <CollapsibleTrigger asChild>
            <button
              type="button"
              className="flex min-w-0 items-center gap-1.5 text-left sm:gap-2"
              aria-expanded={open}
              title={t(open ? 'common.button.collapse.label' : 'common.button.expand.label')}
            >
              {open ? <ChevronDown className="size-3.5 shrink-0 text-muted-foreground" /> : <ChevronRight className="size-3.5 shrink-0 text-muted-foreground" />}
              <span className="min-w-0 truncate text-sm font-medium" data-testid="budget-labels-heading">
                {t('budgets.page.budget.structure.labels.heading')}
              </span>
            </button>
          </CollapsibleTrigger>
          <InfoNote text={t('budgets.page.budget.structure.labels.info')} testId="budget-labels-info-note" />
        </div>
        <CollapsibleContent>
          <ul>
            {labels.map((label) => (
              <LabelRow key={label.id} label={label} currency={currency} opts={opts} onLabelClick={onLabelClick} />
            ))}
          </ul>
        </CollapsibleContent>
      </section>
    </Collapsible>
  )
}

export function BudgetTable({ budget, buckets, renderFolderActions, renderFolderHandle, sectionWrapper, hideChildren, hideContents, ...extras }: BudgetTableProps) {
  const { t } = useTranslation()
  const { data: currencies = [] } = useCurrencies()
  const budgetCurrency = currencies.find((c) => c.id === budget.meta.currencyId)
  const totals = budgetTotals(buckets)
  const actionsColumn = !!extras.renderActions
  const opts = cellOpts(budgetCurrency)
  const accessById = new Map(budget.meta.access.map((a) => [a.user.id, a.user]))

  const realFolders = buckets.withFolder
  const sections: { key: string; name: string; bucket: FolderBucket; folderIndex: number | null }[] = [
    ...realFolders.map((bucket, index) => ({ key: bucket.folder!.id, name: bucket.folder!.name, bucket, folderIndex: index })),
    { key: '__no_folder__', name: t('budgets.page.budget.structure.no_folder'), bucket: buckets.withoutFolder, folderIndex: null },
    { key: '__uncategorized__', name: t('common.uncategorized'), bucket: buckets.uncategorized, folderIndex: null },
    { key: '__archive__', name: t('budgets.page.budget.structure.in_archive'), bucket: buckets.archive, folderIndex: null },
  ]

  return (
    <div className="flex flex-col gap-3" data-testid="budget-table">
      <div className="flex items-center gap-1.5 px-3 text-[11px] uppercase tracking-wide text-muted-foreground sm:gap-2 sm:px-4" data-testid="column-headers">
        <span className="min-w-0 flex-1" />
        <span className="hidden w-24 text-right sm:block">{t('budgets.page.budget.structure.tab.budgeted')}</span>
        <span className="w-20 text-center sm:w-24">{t('budgets.page.budget.structure.tab.spent')}</span>
        <span className="w-20 text-center sm:w-24">{t('budgets.page.budget.structure.tab.available')}</span>
        <span className="hidden w-6 sm:block" />
        {actionsColumn ? <ActionsSpacer /> : null}
      </div>

      {sections.flatMap((section) => {
        // archive and uncategorized are read-only: no drag handle, no folder
        // actions, never a drop container
        const isReadOnlySection = section.key === '__archive__' || section.key === '__uncategorized__'
        // Uncategorized is a single fixed row, not a group: it renders flat,
        // with no header, so the label appears once instead of naming both a
        // section and the lone row inside it. The reporting-tags folder sits
        // right after it -- before Archive -- so it never reads as a breakdown
        // of the Total row further down. Handled ahead of the generic
        // empty-section skip below: the folder must still appear here even in
        // the (common) case where Uncategorized itself has nothing to show for
        // the period.
        if (section.key === '__uncategorized__') {
          const labels = budget.structure.labels ?? []
          // an ephemeral folder: none of the edit-mode props (folder actions,
          // drag handles, section/row wrappers) reach it, so it can never be
          // renamed, moved, deleted, or become a drop target
          const labelsNode =
            labels.length > 0
              ? [<ReportingTagsFolder key="__labels__" labels={labels} currency={budgetCurrency} onLabelClick={extras.onSpentClick} />]
              : []
          if (section.bucket.elements.length === 0) {
            return labelsNode
          }
          return [
            <section key={section.key} className="rounded-md border p-1.5 sm:p-2" data-testid={`budget-folder-${section.name}`}>
              {section.bucket.elements.map((element) => (
                <ElementRow
                  key={element.id}
                  element={element}
                  bucket={section.bucket}
                  budget={budget}
                  currencies={currencies}
                  accessById={accessById}
                  extras={{ onSpentClick: extras.onSpentClick }}
                  actionsColumn={actionsColumn}
                  hideChildren={hideChildren}
                />
              ))}
            </section>,
            ...labelsNode,
          ]
        }
        if (section.bucket.elements.length === 0 && section.folderIndex === null) {
          // both read-only sections hide when they have nothing to show; the
          // empty Default folder survives only in edit mode (folder actions
          // present), where it is the drop target for dragging elements out
          if (isReadOnlySection || realFolders.length === 0 || !renderFolderActions) {
            return []
          }
        }
        const sectionNode = (
          <section key={section.key} className="rounded-md border p-1.5 sm:p-2" data-testid={`budget-folder-${section.name}`}>
            <header className="flex items-center gap-1.5 px-1.5 pb-1 sm:gap-2 sm:px-2">
              {!isReadOnlySection ? renderFolderHandle?.(section.bucket) : null}
              <span className="min-w-0 flex-1 truncate text-sm font-medium" title={section.name}>
                {section.name}
              </span>
              {section.bucket.elements.length > 0 ? (
                <StatCells
                  stats={section.bucket.stats}
                  currency={budgetCurrency}
                  // edit mode: the plus button takes the symbol slot instead
                  hideSymbol={!isReadOnlySection && !!renderFolderActions}
                />
              ) : null}
              {!isReadOnlySection ? renderFolderActions?.(section.bucket, section.folderIndex ?? -1, realFolders.length) : null}
              {isReadOnlySection && actionsColumn ? <ActionsSpacer /> : null}
            </header>
            {hideContents ? null : section.bucket.elements.length === 0 ? (
              <p className="px-2 py-1 text-xs text-muted-foreground">{t('budgets.page.budget.structure.empty_folder.note')}</p>
            ) : (
              section.bucket.elements.map((element) => (
                <ElementRow
                  key={element.id}
                  element={element}
                  bucket={section.bucket}
                  budget={budget}
                  currencies={currencies}
                  accessById={accessById}
                  extras={isReadOnlySection ? { onSpentClick: extras.onSpentClick } : extras}
                  actionsColumn={actionsColumn}
                  hideChildren={hideChildren}
                />
              ))
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

      <div className="hidden items-center gap-2 rounded-md border px-4 py-2 font-medium sm:flex" data-testid="budget-totals">
        <span className="min-w-0 flex-1 truncate text-[15px]">{t('budgets.page.budget.structure.total.name')}</span>
        <span className="w-24 text-right text-[15px] tabular-nums">{moneyFormat(totals.budgeted, budgetCurrency, opts)}</span>
        <span className="w-24 text-center text-[15px] tabular-nums text-muted-foreground">
          {moneyFormat(totals.spent, budgetCurrency, opts)}
        </span>
        <span className="flex w-24 justify-center">
          <AvailablePill available={totals.available} currency={budgetCurrency} />
        </span>
        <span className="w-6 text-center text-xs text-muted-foreground">{budgetCurrency?.symbol}</span>
        {actionsColumn ? <ActionsSpacer /> : null}
      </div>

      {/* the phone table hides the budget column, so the totals unfold into
          labeled lines; the margin keeps the card off the very screen edge */}
      <div
        className="mb-[max(env(safe-area-inset-bottom),0.75rem)] flex flex-col gap-2 rounded-md border px-3 py-2.5 sm:hidden"
        data-testid="budget-totals-mobile"
      >
        <span className="text-[15px] font-medium">{t('budgets.page.budget.structure.total.name')}</span>
        <span className="flex items-baseline justify-between">
          <span className="text-[13px] text-muted-foreground">{t('budgets.page.budget.structure.tab.budgeted')}</span>
          <span className="text-[15px] font-medium tabular-nums">{moneyFormat(totals.budgeted, budgetCurrency, opts)}</span>
        </span>
        <span className="flex items-baseline justify-between">
          <span className="text-[13px] text-muted-foreground">{t('budgets.page.budget.structure.tab.spent')}</span>
          <span className="text-[15px] tabular-nums text-muted-foreground">{moneyFormat(totals.spent, budgetCurrency, opts)}</span>
        </span>
        <span className="flex items-center justify-between">
          <span className="text-[13px] text-muted-foreground">{t('budgets.page.budget.structure.tab.available')}</span>
          <AvailablePill available={totals.available} currency={budgetCurrency} />
        </span>
      </div>
    </div>
  )
}
