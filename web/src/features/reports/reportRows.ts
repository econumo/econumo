import type { BudgetTransactionsParams } from '@/api/budget'
import { UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { ReportRowDto } from '@/api/dto/report'
import type { Id } from '@/api/types'

export interface Found {
  row: ReportRowDto
  parent: ReportRowDto | null
}

// A child row (an envelope's category) carries no plan and no children of its own.
export const asRow = (c: ReportRowDto['children'][number]): ReportRowDto => ({ ...c, ownerUserId: null, planned: [], children: [] })

export function findRow(rows: ReportRowDto[], id: Id | null): Found | null {
  if (!id) return null
  for (const row of rows) {
    if (row.id === id) return { row, parent: null }
    const child = row.children.find((c) => c.id === id)
    if (child) return { row: asRow(child), parent: row }
  }
  return null
}

export interface DrillRow {
  id: Id
  kind: string
  name: string
  side: 'expense' | 'income'
  /** set on an envelope's or a tag's child row */
  parent?: { id: Id; kind: string }
}

export const matches = (value: Id | null, id: Id) => (id === UNCATEGORIZED_ID ? !value : value === id)

// The budget transaction list takes the same selectors the Budget page sends for
// the row; a category listed under a tag is the tag-and-category intersection.
// Uncategorized income has no selector on the server, so it cannot be listed.
export function budgetSelector(row: DrillRow): Omit<BudgetTransactionsParams, 'budgetId' | 'periodStart'> | null {
  const uncategorized = row.id === UNCATEGORIZED_ID
  const underTag = row.parent?.kind === 'tag' ? { tagId: row.parent.id } : {}
  switch (row.kind) {
    case 'category':
      return { ...(uncategorized ? { uncategorized: true } : { categoryId: row.id }), ...underTag }
    case 'tag':
      return { tagId: row.id }
    case 'envelope':
      return { envelopeId: row.id }
    case 'income_category':
      return uncategorized ? null : { income: true, categoryId: row.id }
    case 'income_envelope':
      return { income: true, envelopeId: row.id }
    case 'savings':
      return { accountId: row.id }
    default:
      return null
  }
}

export function spendingRowNames(rows: ReportRowDto[] | undefined, t: (key: string) => string): Record<Id, string> {
  const out: Record<Id, string> = {}
  for (const r of rows ?? []) {
    out[r.id] = rowDisplayName(r, t)
    for (const c of r.children) out[c.id] = rowDisplayName(c, t)
  }
  return out
}


// The server names the no-classification bucket "Uncategorized" whatever the
// grouping; the SPA renders the translated label that fits the grouping.
export function rowDisplayName(row: { id: Id; name: string; kind: string }, t: (key: string) => string): string {
  if (row.id !== UNCATEGORIZED_ID) return row.name
  if (row.kind === 'payee') return t('reports.spending.no_payee')
  if (row.kind === 'tag') return t('reports.spending.no_tag')
  return t('common.uncategorized')
}
