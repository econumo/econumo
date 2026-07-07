import type { ReactNode } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { EntityIcon } from '@/components/EntityIcon'
import { moneyFormat } from '@/lib/money'
import type { MoneyFormatOptions } from '@/lib/money'
import type { BudgetDto, BudgetElementDto } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { UserDto } from '@/api/dto/user'
import { useCurrencies } from '@/features/currencies/queries'
import type { BudgetBuckets, BucketStats, FolderBucket } from './budgetMath'
import { budgetTotals, displayAvailable, displaySpent } from './budgetMath'
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

const cellOpts = (currency: CurrencyDto | undefined): MoneyFormatOptions => ({
  showCurrency: false,
  useNativePrecision: false,
  maxPrecision: currency?.fractionDigits ?? 2,
})

function AvailablePill({ available, currency, testId }: { available: number; currency: CurrencyDto | undefined; testId?: string }) {
  return (
    <span
      data-testid={testId}
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium tabular-nums ${
        available >= 0 ? 'bg-income/10 text-income' : 'bg-expense/10 text-expense'
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
      <span className="w-20 text-center tabular-nums sm:w-24">{moneyFormat(displaySpent(stats.spent), currency, opts)}</span>
      <span className={`w-20 text-center tabular-nums sm:w-24 ${available >= 0 ? 'text-income' : 'text-expense'}`}>
        {moneyFormat(available, currency, opts)}
      </span>
      {hideSymbol ? null : <span className="hidden w-6 text-center sm:block">{currency?.symbol}</span>}
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
  hideChildren = false,
}: {
  element: BudgetElementDto
  bucket: FolderBucket
  budget: BudgetDto
  currencies: CurrencyDto[]
  accessById: Map<string, UserDto>
  extras: ElementRowExtras
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
  const showTransactionsTitle = t('modules.budget.page.budget.structure.element.action.show_transactions')

  const spentCell = (target: BudgetTransactionsTarget, spent: number) =>
    extras.onSpentClick ? (
      <button
        type="button"
        title={showTransactionsTitle}
        aria-label={`transactions ${target.name}`}
        className="w-20 text-center text-[15px] tabular-nums text-muted-foreground underline-offset-2 hover:text-foreground hover:underline sm:w-24"
        onClick={() => extras.onSpentClick!(target)}
      >
        {moneyFormat(displaySpent(spent), currency, opts)}
      </button>
    ) : (
      <span className="w-20 text-center text-[15px] tabular-nums text-muted-foreground sm:w-24">
        {moneyFormat(displaySpent(spent), currency, opts)}
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
      ) : (
        <EntityIcon name={element.icon} className="text-lg text-muted-foreground" />
      )}
      <span className="truncate text-[15px]" title={element.name}>
        {element.name}
      </span>
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
            title={t(unfolded ? 'elements.button.collapse.label' : 'elements.button.expand.label')}
            onClick={() => toggleElement(element.id)}
          >
            {name}
          </button>
        ) : (
          <span className="flex min-w-0 flex-1 items-center gap-2">{name}</span>
        )}
        <span className="hidden w-24 text-right text-[15px] tabular-nums sm:block" data-testid="cell-budgeted">
          {extras.renderBudgetCell ? extras.renderBudgetCell(element) : moneyFormat(element.budgeted, currency, opts)}
        </span>
        <span data-testid="cell-spent" className="flex justify-end">
          {spentCell(
            { id: element.id, type: element.type, name: element.name, icon: element.icon, currencyId: element.currencyId },
            element.spent,
          )}
        </span>
        <span className="flex w-20 justify-center sm:w-24">
          {extras.onAvailableClick ? (
            <button
              type="button"
              title={t('modules.budget.modal.set_limit_form.header')}
              aria-label={`limit ${element.name}`}
              onClick={() => extras.onAvailableClick!(element)}
            >
              <AvailablePill available={available} currency={currency} testId="cell-available" />
            </button>
          ) : (
            <AvailablePill available={available} currency={currency} testId="cell-available" />
          )}
        </span>
        <span className="hidden w-6 text-center text-xs text-muted-foreground sm:block">{currency?.symbol}</span>
        {extras.renderActions?.(element, bucket)}
      </div>
      {expandable && unfolded ? (
        <ul className="pb-1">
          {element.children.map((child) => {
            const owner = accessById.size > 1 && child.ownerUserId ? accessById.get(child.ownerUserId) : undefined
            return (
              <li
                key={child.id}
                className="group flex items-center gap-1.5 rounded-md py-1.5 pl-8 pr-1.5 text-sm text-muted-foreground hover:bg-accent/50 sm:gap-2 sm:pl-12 sm:pr-2"
                data-testid={`child-${child.id}`}
              >
                <EntityIcon name={child.icon} className="text-lg" />
                <span className="min-w-0 flex-1 truncate" title={child.name}>
                  {child.name}
                </span>
                {/* owner sits in the budget column slot, flush under the amounts; row hover only (multi-user budgets) */}
                <span className="hidden w-24 truncate text-right text-xs text-muted-foreground/60 opacity-0 group-hover:opacity-100 sm:block">
                  {owner?.name}
                </span>
                <span data-testid="child-spent" className="flex justify-end">
                  {spentCell({ id: child.id, type: child.type, name: child.name, icon: child.icon, currencyId: element.currencyId }, child.spent)}
                </span>
                <span className="w-20 sm:w-24" />
                <span className="hidden w-6 sm:block" />
              </li>
            )
          })}
        </ul>
      ) : null}
    </div>
  )

  return extras.renderRowWrapper ? <>{extras.renderRowWrapper(element, bucket, row)}</> : row
}

export function BudgetTable({ budget, buckets, renderFolderActions, renderFolderHandle, sectionWrapper, hideChildren, hideContents, ...extras }: BudgetTableProps) {
  const { t } = useTranslation()
  const { data: currencies = [] } = useCurrencies()
  const budgetCurrency = currencies.find((c) => c.id === budget.meta.currencyId)
  const totals = budgetTotals(buckets)
  const opts = cellOpts(budgetCurrency)
  const accessById = new Map(budget.meta.access.map((a) => [a.user.id, a.user]))

  const realFolders = buckets.withFolder
  const sections: { key: string; name: string; bucket: FolderBucket; folderIndex: number | null }[] = [
    ...realFolders.map((bucket, index) => ({ key: bucket.folder!.id, name: bucket.folder!.name, bucket, folderIndex: index })),
    { key: '__no_folder__', name: t('modules.budget.page.budget.structure.no_folder'), bucket: buckets.withoutFolder, folderIndex: null },
    { key: '__archive__', name: t('modules.budget.page.budget.structure.in_archive'), bucket: buckets.archive, folderIndex: null },
  ]

  return (
    <div className="flex flex-col gap-3" data-testid="budget-table">
      <div className="flex items-center gap-1.5 px-3 text-[11px] uppercase tracking-wide text-muted-foreground sm:gap-2 sm:px-4" data-testid="column-headers">
        <span className="min-w-0 flex-1" />
        <span className="hidden w-24 text-right sm:block">{t('modules.budget.page.budget.structure.tab.budgeted')}</span>
        <span className="w-20 text-center sm:w-24">{t('modules.budget.page.budget.structure.tab.spent')}</span>
        <span className="w-20 text-center sm:w-24">{t('modules.budget.page.budget.structure.tab.available')}</span>
        <span className="hidden w-6 sm:block" />
      </div>

      {sections.map((section) => {
        if (section.bucket.elements.length === 0 && section.folderIndex === null) {
          // archive hides when empty; the Default folder stays visible as a drop
          // target whenever real folders exist (Vue renders it unconditionally)
          if (section.key === '__archive__' || realFolders.length === 0) {
            return null
          }
        }
        const isArchiveSection = section.key === '__archive__'
        const sectionNode = (
          <section key={section.key} className="rounded-md border p-1.5 sm:p-2" data-testid={`budget-folder-${section.name}`}>
            <header className="flex items-center gap-1.5 px-1.5 pb-1 sm:gap-2 sm:px-2">
              {!isArchiveSection ? renderFolderHandle?.(section.bucket) : null}
              <span className="min-w-0 flex-1 truncate text-sm font-medium" title={section.name}>
                {section.name}
              </span>
              {section.bucket.elements.length > 0 ? (
                <StatCells
                  stats={section.bucket.stats}
                  currency={budgetCurrency}
                  // edit mode: the plus button takes the symbol slot instead
                  hideSymbol={!isArchiveSection && !!renderFolderActions}
                />
              ) : null}
              {!isArchiveSection ? renderFolderActions?.(section.bucket, section.folderIndex ?? -1, realFolders.length) : null}
            </header>
            {hideContents ? null : section.bucket.elements.length === 0 ? (
              <p className="px-2 py-1 text-xs text-muted-foreground">{t('modules.budget.page.budget.structure.empty_folder.note')}</p>
            ) : (
              section.bucket.elements.map((element) => (
                <ElementRow
                  key={element.id}
                  element={element}
                  bucket={section.bucket}
                  budget={budget}
                  currencies={currencies}
                  accessById={accessById}
                  extras={isArchiveSection ? { onSpentClick: extras.onSpentClick } : extras}
                  hideChildren={hideChildren}
                />
              ))
            )}
          </section>
        )
        return !isArchiveSection && sectionWrapper ? (
          <div key={section.key}>{sectionWrapper(section.bucket, section.key, sectionNode)}</div>
        ) : (
          sectionNode
        )
      })}

      <div className="flex items-center gap-1.5 rounded-md border px-3 py-2 font-medium sm:gap-2 sm:px-4" data-testid="budget-totals">
        <span className="min-w-0 flex-1 truncate text-[15px]">{t('modules.budget.page.budget.structure.total.name')}</span>
        <span className="hidden w-24 text-right text-[15px] tabular-nums sm:block">{moneyFormat(totals.budgeted, budgetCurrency, opts)}</span>
        <span className="w-20 text-center text-[15px] tabular-nums text-muted-foreground sm:w-24">
          {moneyFormat(displaySpent(totals.spent), budgetCurrency, opts)}
        </span>
        <span className="flex w-20 justify-center sm:w-24">
          <AvailablePill available={totals.available} currency={budgetCurrency} />
        </span>
        <span className="hidden w-6 text-center text-xs text-muted-foreground sm:block">{budgetCurrency?.symbol}</span>
      </div>
    </div>
  )
}
