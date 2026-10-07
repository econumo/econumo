import { memo, useState } from 'react'
import type { PointerEvent as ReactPointerEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { EntityIcon } from '@/components/EntityIcon'
import { isZero } from '@/lib/decimal'
import { moneyFormat } from '@/lib/money'
import type { BudgetCommentDto, BudgetMetaDto, PlanChildDto, PlanElementDto } from '@/api/dto/budget'
import { isIncomeType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { elementDisplayName } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'
import { canUpdateLimits, commentCellKey } from './queries'
import { CommentMarker } from './CommentThread'
import { CellShell } from './CellShell'
import { COMMENT_ANCHOR_ATTR } from './cellDom'
import { isEnvelopeType } from './elementEdit'
import { CurrencyTag, RowMenu } from './monthLines'
import type { MenuAction } from './monthLayout'
import { CHILD_INDENT, FOLDER_INDENT, PLAN_FIGURE_COL, PLAN_LINE, PLAN_NAME_COL, PLAN_SELECTED_TINT, ROW_INDENT, useRowLevel } from './monthLayout'
import { planCellView } from './planCell'
import type { PlanRow } from './planMath'

export interface PlanLimitTarget {
  el: PlanElementDto
  month: string
  monthIndex: number
}

/** roving grid selection: col -1 = the row's name cell, 0..visible-1 = month cells.
 *  -1 is the leftmost reachable column: ArrowRight there goes to 0, ArrowLeft at 0
 *  goes to -1, and ArrowLeft AT -1 shifts the window back a month (selection stays
 *  at -1) — the name cell is always reachable by keyboard alone. On a name cell with
 *  children, ArrowRight/ArrowLeft first unfold/fold the breakdown and only then move
 *  on. rowKey may also name a folder header (`pfolder:<id>`), which has just the
 *  name cell but keeps whatever col the selection arrived with. */
export interface PlanSelection {
  rowKey: string
  col: number
}

// A stable DOM id per gridcell, used by aria-activedescendant. rowKey already embeds
// a ':' (id:type), which is valid in an HTML id but not worth relying on downstream, so
// it's sanitized to a safe character set.
export const cellDomId = (rk: string, col: number): string => `plan-cell-${rk.replace(/[^a-zA-Z0-9_-]/g, '_')}-${col}`

const SELECTED_RING = ' ring-2 ring-ring rounded-sm'
export const selectedClass = (selected: boolean): string => (selected ? SELECTED_RING : '')

// an unset cell reads as 0 everywhere else in the grid, so copying/filling from it
// carries an explicit 0 rather than an empty limit
export function sourceAmount(el: PlanElementDto, monthIndex: number): string {
  const planned = monthIndex >= 0 ? (el.cells[monthIndex]?.planned ?? '') : ''
  return planned === '' ? '0' : planned
}

/** A row is editable per-cell only for a non-uncategorized, non-archived parent row,
 *  with a fetched cell and update rights for that month — children never carry limits. */
export function isEditableCell(el: PlanElementDto, month: string, monthIndex: number, meta: BudgetMetaDto, userId: Id | undefined): boolean {
  return el.id !== UNCATEGORIZED_ID && el.isArchived === 0 && monthIndex >= 0 && canUpdateLimits(meta, userId, month)
}

// A comment thread's read/write gate is independent of the role-based `isEditableCell`
// above: a guest may still post, but nobody may write once the budget is archived or
// the month falls outside its start/end range — the thread stays readable either way.
// Exported: the monthly view (BudgetPage) shares this exact rule rather than
// reimplementing it, so both views agree on when a thread is read-only.
export function commentsReadOnly(meta: BudgetMetaDto, month: string): boolean {
  if (meta.isArchived === 1) {
    return true
  }
  if (month < `${meta.startedAt.slice(0, 7)}-01`) {
    return true
  }
  return !!meta.endedAt && month > `${meta.endedAt.slice(0, 7)}-01`
}

export interface GridCtx {
  visibleMonths: string[]
  monthIndex: (m: string) => number
  /** the month selected in the strip: actuals show up to it, plans alone after it */
  selected: string
  /** its column, -1 when the window does not show it; that column is tinted */
  selectedCol: number
  /** a month column too narrow for `actual · plan`: the months before the selected
   *  one show their plan alone */
  showActuals: boolean
  currencies: CurrencyDto[]
  baseCurrencyId: Id
  meta: BudgetMetaDto
  userId: Id | undefined
  isCompact: boolean
  monthLabel: (m: string) => string
  commit: (elementId: Id, month: string, monthIndex: number, amount: string | null) => void
  /** touch viewports: a cell tap opens the item sheet */
  openSheet: (target: PlanLimitTarget) => void
  openTransactions: (el: PlanElementDto, month: string) => void
  commentsByCell: Map<string, BudgetCommentDto[]>
  /** the fetch backing `commentsByCell` hit the 2000-item server cap and dropped the oldest */
  commentsTruncated: boolean
  /** `fromGrid` marks a keyboard-originated open (Shift+Enter / Shift+F2) so the
   *  grid reclaims focus when the thread closes; `anchor` is the cell to pin the
   *  popover to: undefined = look it up from the grid; null = no anchor, open as a
   *  sheet/dialog */
  openComments: (target: PlanLimitTarget, opts?: { fromGrid?: boolean; anchor?: HTMLElement | null }) => void
  /** a thread is open: hover previews stay shut */
  commentsOpen: boolean
  canEdit: boolean
  selection: PlanSelection | null
  select: (rowKey: string, col: number, e?: { target: EventTarget | null }) => void
  fill: {
    active: { rowKey: string; startCol: number; targetCol: number } | null
    start: (rowKey: string, el: PlanElementDto, col: number, e: ReactPointerEvent<HTMLElement>) => void
    move: (e: ReactPointerEvent<HTMLElement>) => void
    end: () => void
    cancel: () => void
  }
  editMode: boolean
  /** a row's ⋮ menu */
  rowMenu?: (el: PlanElementDto) => MenuAction[] | undefined
}

const figureClass = (ctx: GridCtx, col: number): string => `${PLAN_FIGURE_COL}${col === ctx.selectedCol ? ` ${PLAN_SELECTED_TINT}` : ''}`

/** A section's or folder's month: `actual · plan` up to the selected month, plan alone
 *  after it, the same way the rows read. `sum` is null for a month the plan has no
 *  data for. */
export function SumCell({
  index,
  sum,
  month,
  ctx,
  fmt,
}: {
  index: number
  sum: { actual: string; planned: string } | null
  month: string
  ctx: Pick<GridCtx, 'selected' | 'showActuals'>
  fmt: (v: string) => string
}) {
  const actual = sum && (month === ctx.selected || (month < ctx.selected && ctx.showActuals)) ? sum.actual : null
  return (
    <span data-testid={`plan-sum-${index}`} className="text-sm text-muted-foreground tabular-nums">
      {actual !== null ? `${fmt(actual)} · ` : ''}
      {sum && !isZero(sum.planned) ? fmt(sum.planned) : ''}
    </span>
  )
}

// A child is a read-only breakdown of its parent's actuals: it carries no limit and
// is not selectable (no aria-selected, no click/keyboard target) — only rows a limit
// can be set on take the highlight. The indent lives INSIDE the fixed-width name
// column, so the child's figures stay under the parent's.
export const ChildRow = memo(function ChildRow({
  child,
  parentCurrency,
  ctx,
}: {
  child: PlanChildDto
  parentCurrency: CurrencyDto | undefined
  ctx: GridCtx
}) {
  const { t } = useTranslation()
  const level = useRowLevel()
  const displayName = elementDisplayName(child.id, child.name, t)
  const rk = `${child.id}:${child.type}`
  const fmt = (v: string) => moneyFormat(v, parentCurrency, { showCurrency: false, useNativePrecision: false })
  return (
    <div role="row" data-row-id={rk} className={`${PLAN_LINE} min-h-8 text-sm text-muted-foreground hover:bg-accent/50`}>
      <span role="gridcell" className={`${PLAN_NAME_COL} ${CHILD_INDENT[level]}`} title={displayName}>
        <EntityIcon name={child.icon} className="text-lg" />
        <span className="min-w-0 flex-1 truncate">{displayName}</span>
      </span>
      {ctx.visibleMonths.map((m, i) => {
        const idx = ctx.monthIndex(m)
        const cell = idx >= 0 ? child.cells[idx] : undefined
        const actual = cell && (m === ctx.selected || (m < ctx.selected && ctx.showActuals)) ? cell.actual : null
        return (
          <div
            key={m}
            role="gridcell"
            aria-label={t('budgets.page.plan.cell.aria', { name: displayName, month: ctx.monthLabel(m), actual: actual !== null ? fmt(actual) : '—', planned: '—' })}
            data-month={m}
            data-col={i}
            data-testid={`plan-cell-${child.id}:${i}`}
            className={`${figureClass(ctx, i)} py-1`}
          >
            {actual !== null ? (
              <span data-testid="cell-actual" className="text-xs tabular-nums">
                {fmt(actual)}
              </span>
            ) : null}
          </div>
        )
      })}
    </div>
  )
})

export const ElementRow = memo(function ElementRow({ row, ctx }: { row: PlanRow; ctx: GridCtx }) {
  const { t } = useTranslation()
  const level = useRowLevel()
  const el = row.element
  const unfolded = useBudgetPeriodStore((s) => !!s.unfoldedElements[el.id])
  const toggleElement = useBudgetPeriodStore((s) => s.toggleElement)
  const currency = ctx.currencies.find((c) => c.id === el.currencyId)
  const displayName = elementDisplayName(el.id, el.name, t)
  const isUncategorized = el.id === UNCATEGORIZED_ID
  const expandable = el.children.length > 0 || isEnvelopeType(el.type)
  const Chevron = unfolded ? ChevronDown : ChevronRight
  const rk = `${el.id}:${el.type}`
  const nameSelected = ctx.selection?.rowKey === rk && ctx.selection.col === -1
  // The fill handle also shows on the month cell under the mouse, so a value can be
  // dragged right without first clicking the cell to select it. Row-local state, so a
  // hover re-renders this row only, never the grid.
  const [hoverCol, setHoverCol] = useState<number | null>(null)
  const fmt = (v: string) => moneyFormat(v, currency, { showCurrency: false, useNativePrecision: false })
  // the transactions list cannot show income nobody categorized
  const actualLinkable = !(isUncategorized && isIncomeType(el.type))

  return (
    <div data-row-id={rk} className="border-b border-border/60">
      <div role="row" className={`${PLAN_LINE} min-h-9 hover:bg-accent/50`}>
        <div
          role="gridcell"
          id={cellDomId(rk, -1)}
          aria-selected={nameSelected}
          className={`${PLAN_NAME_COL} ${isUncategorized ? `${FOLDER_INDENT} gap-1.5! text-sm text-muted-foreground` : ROW_INDENT[level]} py-1${selectedClass(nameSelected)}`}
          onClick={(e) => ctx.select(rk, -1, e)}
        >
          {/* the name is a selection target only; just the chevron (or ArrowRight/
              ArrowLeft on the highlighted name cell) folds the breakdown, so a click
              meant to highlight the row never springs its children open */}
          {expandable ? (
            <button
              type="button"
              className="flex shrink-0 items-center"
              aria-expanded={unfolded}
              title={t(unfolded ? 'common.button.collapse.label' : 'common.button.expand.label')}
              onClick={() => toggleElement(el.id)}
            >
              <Chevron className="size-3.5 shrink-0 text-muted-foreground" />
            </button>
          ) : (
            <span className="w-3.5 shrink-0" />
          )}
          {isUncategorized ? null : <EntityIcon name={el.icon} className="text-lg text-muted-foreground" />}
          <span className={`min-w-0 truncate ${isUncategorized ? '' : 'text-[15px]'}`} title={displayName}>
            {displayName}
          </span>
          {el.currencyId !== ctx.baseCurrencyId && currency ? <CurrencyTag code={currency.code} /> : null}
          <span className="flex-1" />
          <RowMenu name={displayName} actions={ctx.rowMenu?.(el)} />
        </div>
        {ctx.visibleMonths.map((m, i) => {
          const idx = ctx.monthIndex(m)
          const cell = idx >= 0 ? el.cells[idx] : undefined
          const editable = isEditableCell(el, m, idx, ctx.meta, ctx.userId)
          const view = planCellView({ type: el.type, cell, month: m, selected: ctx.selected })
          const actual = ctx.showActuals || m === ctx.selected ? view.actual : null
          const selected = ctx.selection?.rowKey === rk && ctx.selection.col === i
          const fillSource = ctx.fill.active?.rowKey === rk && ctx.fill.active.startCol === i
          const filled = ctx.fill.active?.rowKey === rk && i > ctx.fill.active.startCol && i <= ctx.fill.active.targetCol
          // an unset cell still edits as 0, so it is draggable too — gating on a set
          // limit would hide the handle on every blank cell. The in-flight drag's
          // source keeps its handle mounted even after the pointer has left the cell:
          // the handle holds the pointer capture, and unmounting it would drop the
          // pointerup that commits the fill.
          const showFillHandle =
            (selected || hoverCol === i || fillSource) && editable && !!cell && !ctx.isCompact && ctx.visibleMonths.length > 1
          const cellComments = ctx.commentsByCell.get(commentCellKey(el.id, m)) ?? []
          const commentCount = cellComments.length
          const target = { el, month: m, monthIndex: idx }
          const actualColor = view.over ? 'text-expense' : 'text-muted-foreground'
          const cellNode = (
            <div
              {...{ [COMMENT_ANCHOR_ATTR]: '' }}
              role="gridcell"
              id={cellDomId(rk, i)}
              aria-selected={selected}
              aria-label={t('budgets.page.plan.cell.aria', {
                name: displayName,
                month: ctx.monthLabel(m),
                actual: actual !== null ? fmt(actual) : '—',
                planned: view.plan !== null ? fmt(view.plan) : '—',
              })}
              data-month={m}
              data-col={i}
              data-testid={`plan-cell-${el.id}:${i}`}
              className={`group/cell relative ${figureClass(ctx, i)} py-1${editable ? ' cursor-pointer' : ''}${hoverCol === i ? ' outline outline-1 outline-border' : ''}${selectedClass(selected)}${filled ? ' fill-covered bg-ring/15' : ''}`}
              onClick={(e) => {
                ctx.select(rk, i, e)
                // touch: the whole cell opens the item sheet; the marker stops its own click
                if (ctx.isCompact && !ctx.editMode && !isUncategorized && idx >= 0) {
                  ctx.openSheet(target)
                }
              }}
              onMouseEnter={() => setHoverCol(i)}
              onMouseLeave={() => setHoverCol((c) => (c === i ? null : c))}
            >
              <span className="flex items-baseline gap-1">
                {actual === null ? null : actualLinkable ? (
                  <button
                    type="button"
                    data-testid="cell-actual"
                    title={t('budgets.page.budget.structure.element.action.show_transactions')}
                    className={`text-xs tabular-nums underline-offset-2 hover:underline ${actualColor}`}
                    onClick={(e) => {
                      // the cell's own click is skipped (a touch tap there opens the
                      // item sheet), but the clicked cell still becomes the selection
                      e.stopPropagation()
                      ctx.select(rk, i, e)
                      ctx.openTransactions(el, m)
                    }}
                  >
                    {fmt(actual)}
                  </button>
                ) : (
                  <span data-testid="cell-actual" className={`text-xs tabular-nums ${actualColor}`}>
                    {fmt(actual)}
                  </span>
                )}
                {actual !== null ? (
                  <span aria-hidden="true" className="text-xs text-muted-foreground/60">
                    ·
                  </span>
                ) : null}
                <span data-testid="cell-planned" className="text-[15px] tabular-nums">
                  {view.plan !== null ? fmt(view.plan) : ''}
                </span>
              </span>
              {(commentCount > 0 || (!ctx.editMode && !commentsReadOnly(ctx.meta, m))) && !isUncategorized ? (
                <CommentMarker count={commentCount} onOpen={(anchor) => ctx.openComments(target, { anchor })} />
              ) : null}
              {showFillHandle ? (
                <span
                  data-testid="fill-handle"
                  role="button"
                  aria-label={t('budgets.page.plan.fill.handle_aria')}
                  className="absolute -right-0.5 -bottom-0.5 z-10 size-2 touch-none cursor-crosshair rounded-[1px] border border-background bg-ring"
                  onPointerDown={(e) => ctx.fill.start(rk, el, i, e)}
                  onPointerMove={(e) => ctx.fill.move(e)}
                  onPointerUp={() => ctx.fill.end()}
                  onPointerCancel={() => ctx.fill.cancel()}
                  onClick={(e) => e.stopPropagation()}
                />
              ) : null}
            </div>
          )
          return (
            <CellShell
              key={m}
              comments={isUncategorized ? [] : cellComments}
              previewDisabled={ctx.commentsOpen || ctx.editMode}
              shortcutDisabled={ctx.editMode}
              onOpenComments={isUncategorized ? undefined : (anchor) => ctx.openComments(target, { anchor })}
            >
              {cellNode}
            </CellShell>
          )
        })}
      </div>
      {expandable && unfolded ? (
        <div>
          {el.children.length === 0 ? (
            <p className={`${CHILD_INDENT[level]} px-2 py-1 text-xs text-muted-foreground`}>{t('budgets.page.budget.structure.empty_envelope.note')}</p>
          ) : null}
          {el.children.map((child) => (
            <ChildRow key={child.id} child={child} parentCurrency={currency} ctx={ctx} />
          ))}
        </div>
      ) : null}
    </div>
  )
})
