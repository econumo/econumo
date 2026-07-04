import type { ReactNode } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { EntityIcon } from '@/components/EntityIcon'
import { moneyFormat } from '@/lib/money'
import type { BudgetDto, BudgetElementDto } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import { useCurrencies } from '@/features/currencies/queries'
import type { BudgetBuckets, BucketStats, FolderBucket } from './budgetMath'
import { budgetTotals, displayAvailable, displaySpent } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'

export interface ElementRowExtras {
  /** the budget cell contents (set-limit editor) — defaults to a plain value */
  renderBudgetCell?: (element: BudgetElementDto) => ReactNode
  /** trailing actions (edit-mode menus, drag handle) */
  renderActions?: (element: BudgetElementDto, bucket: FolderBucket) => ReactNode
  renderRowWrapper?: (element: BudgetElementDto, bucket: FolderBucket, row: ReactNode) => ReactNode
  onElementClick?: (element: BudgetElementDto) => void
}

interface BudgetTableProps extends ElementRowExtras {
  budget: BudgetDto
  buckets: BudgetBuckets
  renderFolderActions?: (bucket: FolderBucket, index: number, total: number) => ReactNode
  /** wraps folder/no-folder sections (dnd droppables in edit mode) */
  sectionWrapper?: (bucket: FolderBucket, sectionKey: string, node: ReactNode) => ReactNode
}

function StatLine({ stats, currency }: { stats: BucketStats; currency: CurrencyDto | undefined }) {
  const { t } = useTranslation()
  const opts = { showCurrency: false, useNativePrecision: false } as const
  return (
    <span className="flex gap-3 text-xs text-muted-foreground" data-testid="stat-line">
      <span>
        {t('modules.budget.page.budget.structure.tab.budgeted')} {moneyFormat(stats.budgeted, currency, opts)}
      </span>
      <span>
        {t('modules.budget.page.budget.structure.tab.spent')} {moneyFormat(displaySpent(stats.spent), currency, opts)}
      </span>
      <span>
        {t('modules.budget.page.budget.structure.tab.available')} {moneyFormat(stats.available, currency, opts)}
      </span>
    </span>
  )
}

function ElementRow({
  element,
  bucket,
  budget,
  currencies,
  extras,
}: {
  element: BudgetElementDto
  bucket: FolderBucket
  budget: BudgetDto
  currencies: CurrencyDto[]
  extras: ElementRowExtras
}) {
  const unfolded = useBudgetPeriodStore((s) => !!s.unfoldedElements[element.id])
  const toggleElement = useBudgetPeriodStore((s) => s.toggleElement)

  const currencyId = element.currencyId ?? budget.meta.currencyId
  const currency = currencies.find((c) => c.id === currencyId)
  const available = displayAvailable(element)
  const expandable = element.children.length > 0
  const opts = { showCurrency: false, useNativePrecision: false } as const

  const row = (
    <div className="flex flex-col" data-testid={`element-${element.id}`}>
      <div className="flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-accent/50">
        <button
          type="button"
          className="flex min-w-0 flex-1 items-center gap-2 text-left"
          aria-expanded={expandable ? unfolded : undefined}
          onClick={() => (expandable ? toggleElement(element.id) : extras.onElementClick?.(element))}
        >
          {expandable ? (
            unfolded ? <ChevronDown className="size-3.5 shrink-0 text-muted-foreground" /> : <ChevronRight className="size-3.5 shrink-0 text-muted-foreground" />
          ) : (
            <span className="w-3.5 shrink-0" />
          )}
          <EntityIcon name={element.icon} className="text-base text-muted-foreground" />
          <span className="truncate text-sm" title={element.name}>
            {element.name}
          </span>
        </button>
        <span className="w-24 text-right text-sm tabular-nums" data-testid="cell-budgeted">
          {extras.renderBudgetCell ? extras.renderBudgetCell(element) : moneyFormat(element.budgeted, currency, opts)}
        </span>
        <span className="w-24 text-right text-sm tabular-nums text-muted-foreground" data-testid="cell-spent">
          {moneyFormat(displaySpent(element.spent), currency, opts)}
        </span>
        <span
          className={`w-24 text-right text-sm tabular-nums ${available >= 0 ? 'text-green-600' : 'text-red-600'}`}
          data-testid="cell-available"
        >
          {moneyFormat(available, currency, opts)}
        </span>
        <span className="w-6 text-right text-xs text-muted-foreground">{currency?.symbol}</span>
        {extras.renderActions?.(element, bucket)}
      </div>
      {expandable && unfolded ? (
        <ul className="pb-1 pl-12">
          {element.children.map((child) => (
            <li key={child.id} className="flex items-center gap-2 py-1 text-sm text-muted-foreground" data-testid={`child-${child.id}`}>
              <EntityIcon name={child.icon} className="text-sm" />
              <span className="min-w-0 flex-1 truncate">{child.name}</span>
              <span className="tabular-nums">{moneyFormat(displaySpent(child.spent), currency, opts)}</span>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )

  return extras.renderRowWrapper ? <>{extras.renderRowWrapper(element, bucket, row)}</> : row
}

export function BudgetTable({ budget, buckets, renderFolderActions, sectionWrapper, ...extras }: BudgetTableProps) {
  const { t } = useTranslation()
  const { data: currencies = [] } = useCurrencies()
  const budgetCurrency = currencies.find((c) => c.id === budget.meta.currencyId)
  const totals = budgetTotals(buckets)
  const opts = { showCurrency: false, useNativePrecision: false } as const

  const realFolders = buckets.withFolder
  const sections: { key: string; name: string; bucket: FolderBucket; folderIndex: number | null }[] = [
    ...realFolders.map((bucket, index) => ({ key: bucket.folder!.id, name: bucket.folder!.name, bucket, folderIndex: index })),
    { key: '__no_folder__', name: t('modules.budget.page.budget.structure.no_folder'), bucket: buckets.withoutFolder, folderIndex: null },
    { key: '__archive__', name: t('modules.budget.page.budget.structure.in_archive'), bucket: buckets.archive, folderIndex: null },
  ]

  return (
    <div className="flex flex-col gap-3" data-testid="budget-table">
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
          <section key={section.key} className="rounded-md border p-2" data-testid={`budget-folder-${section.name}`}>
            <header className="flex items-center gap-2 px-2 pb-1">
              <span className="flex-1 truncate text-sm font-medium" title={section.name}>
                {section.name}
              </span>
              {section.bucket.elements.length > 0 ? <StatLine stats={section.bucket.stats} currency={budgetCurrency} /> : null}
              {!isArchiveSection && section.folderIndex !== null
                ? renderFolderActions?.(section.bucket, section.folderIndex, realFolders.length)
                : null}
            </header>
            {section.bucket.elements.length === 0 ? (
              <p className="px-2 py-1 text-xs text-muted-foreground">{t('modules.budget.page.budget.structure.empty_folder.note')}</p>
            ) : (
              section.bucket.elements.map((element) => (
                <ElementRow
                  key={element.id}
                  element={element}
                  bucket={section.bucket}
                  budget={budget}
                  currencies={currencies}
                  extras={isArchiveSection ? { onElementClick: extras.onElementClick } : extras}
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

      <div className="flex items-center gap-2 rounded-md border px-4 py-2 font-medium" data-testid="budget-totals">
        <span className="min-w-0 flex-1 truncate text-sm">{t('modules.budget.page.budget.structure.total.name')}</span>
        <span className="w-24 text-right text-sm tabular-nums">{moneyFormat(totals.budgeted, budgetCurrency, opts)}</span>
        <span className="w-24 text-right text-sm tabular-nums text-muted-foreground">
          {moneyFormat(displaySpent(totals.spent), budgetCurrency, opts)}
        </span>
        <span className={`w-24 text-right text-sm tabular-nums ${totals.available >= 0 ? 'text-green-600' : 'text-red-600'}`}>
          {moneyFormat(totals.available, budgetCurrency, opts)}
        </span>
        <span className="w-6 text-right text-xs text-muted-foreground">{budgetCurrency?.symbol}</span>
      </div>
    </div>
  )
}
