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
import { DragChild, EnvelopeDrop, EnvelopeHeadDrop } from './MonthDrag'
import { CurrencyTag, Dash, RowMenu } from './monthLines'
import type { MenuAction } from './monthLayout'
import { CHILD_INDENT, FOLDER_INDENT, PLAN_CROSSHAIR, PLAN_FIGURE_COL, PLAN_LINE, PLAN_NAME_COL, PLAN_SELECTED_TINT, ROW_INDENT, useRowLevel } from './monthLayout'
import { planCellView, shownActual } from './planCell'
import { PlanCellInput } from './PlanCellInput'
import type { CellMove } from './PlanCellInput'
import type { PlanRow } from './planMath'

export interface PlanLimitTarget {
  el: PlanElementDto
  month: string
  monthIndex: number
}

/** roving grid selection: a row's month cell (col 0..visible-1). Names and section or
 *  folder lines are not stops; ← on the first month and → on the last page the window. */
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

/** the cell whose in-cell editor is open. `month` pins it: the editor stays on the
 *  month it was opened for even if the window moves under it. */
export interface PlanCellEdit {
  rowKey: string
  col: number
  month: string
  initial: string
  replace: boolean
}

export interface GridCtx {
  visibleMonths: string[]
  monthIndex: (m: string) => number
  /** the month selected in the strip: actuals show up to it, plans alone after it */
  selected: string
  /** its column, -1 when the window does not show it; that column is tinted */
  selectedCol: number
  /** the selected cell's column, highlighted down the grid; -1 when nothing is selected */
  crosshairCol: number
  currencies: CurrencyDto[]
  baseCurrencyId: Id
  meta: BudgetMetaDto
  userId: Id | undefined
  isCompact: boolean
  monthLabel: (m: string) => string
  /** touch viewports: a cell tap opens the item sheet */
  openSheet: (target: PlanLimitTarget) => void
  openTransactions: (el: PlanElementDto, month: string) => void
  commentsByCell: Map<string, BudgetCommentDto[]>
  /** `fromGrid` marks a keyboard-originated open (Shift+Enter / Shift+F2) so the
   *  grid reclaims focus when the thread closes; `anchor` is the cell to pin the
   *  popover to: undefined = look it up from the grid; null = no anchor, open as a
   *  sheet/dialog */
  openComments: (target: PlanLimitTarget, opts?: { fromGrid?: boolean; anchor?: HTMLElement | null }) => void
  /** a thread is open: hover previews stay shut */
  commentsOpen: boolean
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
  /** rows, folders and envelope categories drag by their grips */
  drag: boolean
  /** the element being dragged: its own categories fold for the gesture */
  dragActiveId: string | null
  editing: PlanCellEdit | null
  /** opens the in-cell editor on an editable month cell (desktop only); `text` is the
   *  typed character that replaces the value */
  startEdit: (rowKey: string, col: number, opts: { replace: boolean; text?: string }) => void
  finishEdit: (raw: string, move: CellMove) => void
  cancelEdit: () => void
  /** a row's ⋮ menu; undefined while the line controls are off (touch, no edit mode) */
  rowMenu?: (el: PlanElementDto) => MenuAction[] | undefined
  /** the ⋮ menu of a category inside an envelope */
  childMenu?: (child: PlanChildDto) => MenuAction[] | undefined
}

const figureClass = (ctx: GridCtx, col: number): string =>
  `${PLAN_FIGURE_COL}${col === ctx.selectedCol ? ` ${PLAN_SELECTED_TINT}` : ''}${col === ctx.crosshairCol ? ` ${PLAN_CROSSHAIR}` : ''}`

/** A section's or folder's month: `actual · plan` up to the selected month, plan alone
 *  after it, the same way the rows read. `sum` is null for a month the plan has no
 *  data for. */
export function SumCell({
  index,
  sum,
  month,
  ctx,
  fmt,
  balance = false,
}: {
  index: number
  sum: { actual: string; planned: string } | null
  month: string
  ctx: Pick<GridCtx, 'selected'>
  fmt: (v: string) => string
  /** the Savings line: its figure is the total month-end balance, in every month */
  balance?: boolean
}) {
  const plan = sum && !isZero(sum.planned) ? sum.planned : null
  const { actual, dash } = balance
    ? { actual: sum ? sum.actual : null, dash: false }
    : shownActual(sum && month <= ctx.selected ? sum.actual : null, plan !== null)
  return (
    <span data-testid={`plan-sum-${index}`} className="flex min-w-0 items-baseline gap-1 text-muted-foreground tabular-nums">
      {actual !== null ? (
        <span className="min-w-0 truncate text-xs" title={fmt(actual)}>
          {fmt(actual)}
        </span>
      ) : dash ? (
        <span className="text-xs">
          <Dash />
        </span>
      ) : null}
      {(actual !== null || dash) && plan !== null ? <FigureDot /> : null}
      {plan !== null ? <span className="shrink-0 text-sm whitespace-nowrap">{fmt(plan)}</span> : null}
    </span>
  )
}

function FigureDot() {
  return (
    <span aria-hidden="true" className="shrink-0 text-xs text-muted-foreground/60">
      ·
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
  menu,
}: {
  child: PlanChildDto
  parentCurrency: CurrencyDto | undefined
  ctx: GridCtx
  menu?: MenuAction[]
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
        <RowMenu name={displayName} actions={menu} />
      </span>
      {ctx.visibleMonths.map((m, i) => {
        const idx = ctx.monthIndex(m)
        const cell = idx >= 0 ? child.cells[idx] : undefined
        const { actual } = shownActual(cell && m <= ctx.selected ? cell.actual : null, false)
        return (
          <div
            key={m}
            role="gridcell"
            aria-label={t('budgets.page.plan.cell.aria', { name: displayName, month: ctx.monthLabel(m), actual: actual !== null ? fmt(actual) : '—', planned: '—' })}
            data-month={m}
            data-col={i}
            data-testid={`plan-cell-${child.id}:${i}`}
            data-crosshair={i === ctx.crosshairCol ? 'col' : undefined}
            className={`${figureClass(ctx, i)} py-1`}
          >
            {actual !== null ? (
              <span data-testid="cell-actual" className="min-w-0 truncate text-xs tabular-nums" title={fmt(actual)}>
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
  const open = expandable && unfolded && ctx.dragActiveId !== el.id
  // a live envelope takes categories dropped on it and lets its own be dragged out
  const childDrag = ctx.drag && isEnvelopeType(el.type) && el.isArchived === 0
  const Chevron = unfolded ? ChevronDown : ChevronRight
  const rk = `${el.id}:${el.type}`
  // The fill handle also shows on the month cell under the mouse, so a value can be
  // dragged right without first clicking the cell to select it. Row-local state, so a
  // hover re-renders this row only, never the grid.
  const [hoverCol, setHoverCol] = useState<number | null>(null)
  const fmt = (v: string) => moneyFormat(v, currency, { showCurrency: false, useNativePrecision: false })
  // the transactions list cannot show income nobody categorized
  const actualLinkable = !(isUncategorized && isIncomeType(el.type))
  const childList = open ? (
    <div>
      {el.children.length === 0 ? (
        <p className={`${CHILD_INDENT[level]} px-2 py-1 text-xs text-muted-foreground`}>{t('budgets.page.budget.structure.empty_envelope.note')}</p>
      ) : null}
      {el.children.map((child) => {
        const line = <ChildRow key={child.id} child={child} parentCurrency={currency} ctx={ctx} menu={isEnvelopeType(el.type) ? ctx.childMenu?.(child) : undefined} />
        return childDrag ? (
          <DragChild key={child.id} id={child.id}>
            {line}
          </DragChild>
        ) : (
          line
        )
      })}
    </div>
  ) : null

  return (
    <div data-row-id={rk} data-plan-line="" className="border-b border-border/60">
      <div
        role="row"
        data-crosshair={ctx.selection?.rowKey === rk ? 'row' : undefined}
        className={`${PLAN_LINE} relative min-h-9 hover:bg-accent/50${ctx.selection?.rowKey === rk ? ` ${PLAN_CROSSHAIR}` : ''}`}
      >
        {/* the name is not a cell the cursor visits: the keyboard walks the months */}
        <div
          role="rowheader"
          className={`${PLAN_NAME_COL} ${isUncategorized ? `${FOLDER_INDENT} gap-1.5! text-sm text-muted-foreground` : ROW_INDENT[level]} py-1`}
        >
          {/* only the chevron folds the breakdown: a click elsewhere on the name
              does nothing */}
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
          const actual = view.actual
          const selected = ctx.selection?.rowKey === rk && ctx.selection.col === i
          const fillSource = ctx.fill.active?.rowKey === rk && ctx.fill.active.startCol === i
          const filled = ctx.fill.active?.rowKey === rk && i > ctx.fill.active.startCol && i <= ctx.fill.active.targetCol
          // an unset cell still edits as 0, so it is draggable too — gating on a set
          // limit would hide the handle on every blank cell. The in-flight drag's
          // source keeps its handle mounted even after the pointer has left the cell:
          // the handle holds the pointer capture, and unmounting it would drop the
          // pointerup that commits the fill.
          const editing = ctx.editing?.rowKey === rk && ctx.editing.month === m ? ctx.editing : null
          const showFillHandle =
            !editing && (selected || hoverCol === i || fillSource) && editable && !!cell && !ctx.isCompact && ctx.visibleMonths.length > 1
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
              data-crosshair={i === ctx.crosshairCol ? 'col' : undefined}
              className={`group/cell relative ${figureClass(ctx, i)} py-1${editable ? ' cursor-pointer' : ''}${hoverCol === i ? ' outline outline-1 outline-border' : ''}${selectedClass(selected)}${filled ? ' fill-covered bg-ring/15' : ''}`}
              onClick={(e) => {
                ctx.select(rk, i, e)
                // touch: the whole cell opens the item sheet; the marker stops its own click
                if (ctx.isCompact && !ctx.editMode && !isUncategorized && idx >= 0) {
                  ctx.openSheet(target)
                }
              }}
              onDoubleClick={(e) => {
                // the actual opens transactions and the corner opens comments: a
                // double-click on either acts on that control, not on the plan
                if (!(e.target as HTMLElement).closest('button, [role="button"], input')) {
                  ctx.startEdit(rk, i, { replace: false })
                }
              }}
              onMouseEnter={() => setHoverCol(i)}
              onMouseLeave={() => setHoverCol((c) => (c === i ? null : c))}
            >
              {/* the plan never gives way: when a column runs out of room only the
                  actual is cut, its full figure kept in the tooltip. A savings balance
                  keeps to the cell's left edge, so balances line up down the column;
                  while the plan is edited the actual stays in view beside the editor. */}
              <span
                data-testid="cell-figures"
                className={`flex min-w-0 gap-1${view.balance || editing ? ' flex-1 justify-between' : ''} ${editing ? 'items-center' : 'items-baseline'}`}
              >
                {actual === null ? (
                  view.dash ? (
                    <span data-testid="cell-no-actual" className="text-xs">
                      <Dash />
                    </span>
                  ) : null
                ) : actualLinkable && (!view.balance || m <= ctx.selected) ? (
                  <button
                    type="button"
                    data-testid="cell-actual"
                    data-figure={view.balance ? 'balance' : undefined}
                    title={`${fmt(actual)}. ${view.balance ? `${t('budgets.page.savings.balance_hint')} ` : ''}${t('budgets.page.budget.structure.element.action.show_transactions')}`}
                    className={`min-w-0 truncate text-xs tabular-nums underline-offset-2 hover:underline ${actualColor}`}
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
                  <span
                    data-testid="cell-actual"
                    data-figure={view.balance ? 'balance' : undefined}
                    className={`min-w-0 truncate text-xs tabular-nums ${actualColor}`}
                    title={view.balance ? `${fmt(actual)}. ${t('budgets.page.savings.balance_hint')}` : fmt(actual)}
                  >
                    {fmt(actual)}
                  </span>
                )}
                {editing ? (
                  <span className="w-[58%] min-w-16 shrink-0">
                    <PlanCellInput
                      initial={editing.initial}
                      label={t('budgets.page.plan.cell.edit_aria', { name: displayName, month: ctx.monthLabel(m) })}
                      onCommit={ctx.finishEdit}
                      onCancel={ctx.cancelEdit}
                    />
                  </span>
                ) : view.balance ? (
                  // the cell's two ends already keep balance and plan apart: no dot
                  <span data-testid="cell-planned" className="shrink-0 text-[15px] whitespace-nowrap tabular-nums">
                    {view.plan !== null ? fmt(view.plan) : <Dash />}
                  </span>
                ) : (
                  <>
                    {(actual !== null || view.dash) && view.plan !== null ? <FigureDot /> : null}
                    <span data-testid="cell-planned" className="shrink-0 text-[15px] whitespace-nowrap tabular-nums">
                      {view.plan !== null ? fmt(view.plan) : ''}
                    </span>
                  </>
                )}
              </span>
              {!editing && (commentCount > 0 || (!ctx.editMode && !commentsReadOnly(ctx.meta, m))) && !isUncategorized ? (
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
              previewDisabled={ctx.commentsOpen || ctx.editMode || !!editing}
              shortcutDisabled={ctx.editMode}
              onOpenComments={isUncategorized ? undefined : (anchor) => ctx.openComments(target, { anchor })}
            >
              {cellNode}
            </CellShell>
          )
        })}
        {childDrag && !open ? <EnvelopeHeadDrop envelopeId={el.id} /> : null}
      </div>
      {open ? (childDrag ? <EnvelopeDrop envelopeId={el.id}>{childList}</EnvelopeDrop> : childList) : null}
    </div>
  )
})
