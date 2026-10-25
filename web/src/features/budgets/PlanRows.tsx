import { memo, useState } from 'react'
import type { PointerEvent as ReactPointerEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { EntityIcon } from '@/components/EntityIcon'
import { isZero } from '@/lib/decimal'
import { moneyFormat } from '@/lib/money'
import { pluralPick } from '@/lib/plural'
import { METRICS, trackEvent } from '@/lib/metrics'
import type { ReactNode } from 'react'
import type { BudgetCommentDto, BudgetMetaDto, PlanChildDto, PlanElementDto } from '@/api/dto/budget'
import { BudgetElementType, UNCATEGORIZED_ID } from '@/api/dto/budget'
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
import { CurrencyTag, Dash, FigureCells, RowMenu } from './monthLines'
import type { MenuAction } from './monthLayout'
import { CHILD_INDENT, FOLDER_INDENT, FUTURE_FIGURE, groupedLevel, PLAN_CROSSHAIR, PLAN_FIGURE_COL, PLAN_LINE, PLAN_NAME_COL, PLAN_SELECTED_TINT, ROW_INDENT, RowLevelContext, useRowLevel } from './monthLayout'
import { planCellView, shownActual } from './planCell'
import { PlanCellInput } from './PlanCellInput'
import type { CellMove } from './PlanCellInput'
import type { MonthExchange, PlanRow } from './planMath'
import { tagGroupPlanSums } from './planMath'

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

export const pickedKey = (rk: string, col: number): string => `${rk}|${col}`

// an unset cell reads as 0 everywhere else in the grid, so copying/filling from it
// carries an explicit 0 rather than an empty limit
export function sourceAmount(el: PlanElementDto, monthIndex: number): string {
  const planned = monthIndex >= 0 ? (el.cells[monthIndex]?.planned ?? '') : ''
  return planned === '' ? '0' : planned
}

/** A row is editable per-cell only for a non-uncategorized parent row, with a fetched
 *  cell and update rights for that month — children never carry limits. An archived
 *  category, tag or envelope keeps an editable plan, so its history can be corrected
 *  (the totals still leave it out); a deleted savings account's cannot. */
export function isEditableCell(el: PlanElementDto, month: string, monthIndex: number, meta: BudgetMetaDto, userId: Id | undefined): boolean {
  if (el.isArchived === 1 && el.type === BudgetElementType.SAVINGS) {
    return false
  }
  return el.id !== UNCATEGORIZED_ID && monthIndex >= 0 && canUpdateLimits(meta, userId, month)
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
  /** into the budget currency, at each window month's rates */
  exchange: MonthExchange
  /** the current month: actuals show up to it, plans alone after it */
  selected: string
  /** a tinted column, -1 for none (the Plan grid tints none) */
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
  /** desktop: every selected cell (pickedKey) once more than one is; null for one */
  picked: Set<string> | null
  /** desktop: Shift+click spans the rectangle from the active cell, Ctrl/⌘+click
   *  adds or removes one cell */
  pick: (rowKey: string, col: number, mode: 'range' | 'toggle') => void
  /** desktop: a mouse drag over the month cells selects the rectangle it spans */
  dragSelect: {
    active: boolean
    start: (rowKey: string, col: number) => void
    over: (rowKey: string, col: number, buttons: number) => void
  }
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

/** A later savings balance links to its month's transactions only when money is due
 *  to move then: the list holds nothing but those upcoming payments. */
function movesMoney(cell: PlanElementDto['cells'][number] | undefined): boolean {
  return !!cell && (!isZero(cell.actual) || !isZero(cell.upcoming ?? '0'))
}

/** A later month's actual shows only when upcoming money folded one in, lighter, as
 *  the rows read; up to the current month a zero reads as a dash next to a plan. */
function laterActual(actual: string | null, month: string, current: string): string | null {
  return actual !== null && month > current && isZero(actual) ? null : actual
}

/** A section's or folder's month: `actual · plan` up to the current month, after it
 *  the plan plus any folded upcoming actual, the same way the rows read. `sum` is
 *  null for a month the plan has no data for. */
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
    : shownActual(laterActual(sum ? sum.actual : null, month, ctx.selected), plan !== null)
  const later = !balance && month > ctx.selected
  return (
    <span data-testid={`plan-sum-${index}`} className="flex min-w-0 flex-1 items-baseline justify-between gap-1 text-muted-foreground tabular-nums">
      {actual !== null ? (
        <span className={`min-w-0 truncate text-xs${later ? ` ${FUTURE_FIGURE}` : ''}`} title={fmt(actual)}>
          {fmt(actual)}
        </span>
      ) : dash ? (
        <span className="text-xs">
          <Dash />
        </span>
      ) : null}
      {plan !== null ? <span className="ml-auto shrink-0 text-sm whitespace-nowrap">{fmt(plan)}</span> : null}
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
        const { actual } = shownActual(laterActual(cell ? cell.actual : null, m, ctx.selected), false)
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
            <span className="flex min-w-0 flex-1">
              {actual !== null ? (
                <span data-testid="cell-actual" className={`min-w-0 truncate text-xs tabular-nums${m > ctx.selected ? ` ${FUTURE_FIGURE}` : ''}`} title={fmt(actual)}>
                  {fmt(actual)}
                </span>
              ) : null}
            </span>
          </div>
        )
      })}
    </div>
  )
})

/** Same-name tags (each partner's own "Travel") under one line with their monthly
 *  sums. The line is no keyboard stop and carries no plan: each tag keeps its own. */
export function PlanTagGroup({ groupKey, name, rows, ctx, children }: { groupKey: string; name: string; rows: PlanRow[]; ctx: GridCtx; children: ReactNode }) {
  const { t, i18n } = useTranslation()
  const level = useRowLevel()
  const open = useBudgetPeriodStore((s) => !!s.planUnfoldedElements[groupKey])
  const togglePlanElement = useBudgetPeriodStore((s) => s.togglePlanElement)
  const { sums, currencyId } = tagGroupPlanSums(
    rows.map((r) => r.element),
    ctx.visibleMonths,
    ctx.monthIndex,
    ctx.exchange,
    ctx.baseCurrencyId,
  )
  const currency = ctx.currencies.find((c) => c.id === currencyId)
  const fmt = (v: string) => moneyFormat(v, currency, { showCurrency: false, useNativePrecision: false })
  const Chevron = open ? ChevronDown : ChevronRight
  return (
    <div data-testid={`plan-tag-group-${groupKey}`}>
      <div role="row" data-plan-line="" className="border-b border-border/60">
        <div className={`${PLAN_LINE} relative min-h-9 hover:bg-accent/50`}>
          <div role="rowheader" className={`${PLAN_NAME_COL} ${ROW_INDENT[level]} py-1`}>
            <button
              type="button"
              className="flex min-w-0 items-center gap-2 text-left"
              aria-expanded={open}
              title={t(open ? 'common.button.collapse.label' : 'common.button.expand.label')}
              onClick={() => {
                if (!open) {
                  trackEvent(METRICS.BUDGET_TAG_GROUP_EXPAND, { view: 'plan', tags: rows.length })
                }
                togglePlanElement(groupKey)
              }}
            >
              <Chevron className="size-3.5 shrink-0 text-muted-foreground" />
              <EntityIcon name="tag" className="text-lg text-muted-foreground" />
              <span className="min-w-0 truncate text-[15px]" title={name}>
                {name}
              </span>
              <span className="shrink-0 text-xs text-muted-foreground">{pluralPick(t('budgets.page.budget.structure.tag_group.count'), rows.length, i18n.language)}</span>
            </button>
            {currencyId !== ctx.baseCurrencyId && currency ? <CurrencyTag code={currency.code} /> : null}
          </div>
          <FigureCells
            cells={ctx.visibleMonths.map((m, i) => (
              <SumCell key={m} index={i} sum={ctx.monthIndex(m) >= 0 ? (sums[i] ?? null) : null} month={m} ctx={ctx} fmt={fmt} />
            ))}
          />
        </div>
      </div>
      {open ? <RowLevelContext.Provider value={groupedLevel(level)}>{children}</RowLevelContext.Provider> : null}
    </div>
  )
}

export const ElementRow = memo(function ElementRow({ row, ctx, grouped = false }: { row: PlanRow; ctx: GridCtx; grouped?: boolean }) {
  const { t } = useTranslation()
  const level = useRowLevel()
  const el = row.element
  // inside a same-name group the names match: the owner tells the tags apart
  const ownerHint = grouped && el.ownerUserId ? ctx.meta.access.find((a) => a.user.id === el.ownerUserId)?.user.name : undefined
  const unfolded = useBudgetPeriodStore((s) => !!s.planUnfoldedElements[el.id])
  const togglePlanElement = useBudgetPeriodStore((s) => s.togglePlanElement)
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
              onClick={() => togglePlanElement(el.id)}
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
          {ownerHint ? <span className="shrink-0 truncate text-xs text-muted-foreground">{ownerHint}</span> : null}
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
          const picked = !!ctx.picked?.has(pickedKey(rk, i))
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
          const actualColor = view.over ? 'text-expense' : m > ctx.selected && !view.balance ? FUTURE_FIGURE : 'text-muted-foreground'
          const cellNode = (
            <div
              {...{ [COMMENT_ANCHOR_ATTR]: '' }}
              role="gridcell"
              id={cellDomId(rk, i)}
              aria-selected={selected || picked}
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
              className={`group/cell relative ${figureClass(ctx, i)} py-1${editable ? ' cursor-pointer' : ''}${hoverCol === i ? ' outline outline-1 outline-border' : ''}${selectedClass(selected)}${picked ? ' bg-ring/15' : ''}${filled ? ' fill-covered bg-ring/15' : ''}`}
              onMouseDown={(e) => {
                // Shift+click would otherwise also select the page's text in between;
                // an open editor keeps the press, so its blur still commits it
                if (!ctx.isCompact && e.shiftKey && !ctx.editing) {
                  e.preventDefault()
                }
              }}
              onPointerDown={(e) => {
                if (
                  !ctx.isCompact &&
                  e.button === 0 &&
                  e.pointerType !== 'touch' &&
                  !e.shiftKey &&
                  !e.ctrlKey &&
                  !e.metaKey &&
                  !(e.target as HTMLElement).closest('button, [role="button"], input')
                ) {
                  ctx.dragSelect.start(rk, i)
                }
              }}
              onPointerEnter={(e) => ctx.dragSelect.over(rk, i, e.buttons)}
              onClick={(e) => {
                if (!ctx.isCompact && (e.shiftKey || e.ctrlKey || e.metaKey)) {
                  ctx.pick(rk, i, e.shiftKey ? 'range' : 'toggle')
                  return
                }
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
              {/* one layout in every month, edited or not: the actual (a balance, a dash)
                  at the cell's left edge, the plan — or its editor — at the right. Nothing
                  shifts when editing starts. The plan never gives way: when a column runs
                  out of room only the actual is cut, its full figure in the tooltip. */}
              <span
                data-testid="cell-figures"
                className={`flex min-w-0 flex-1 justify-between gap-1 ${editing ? 'items-center' : 'items-baseline'}`}
              >
                {actual === null && view.upcoming ? (
                  <button
                    type="button"
                    data-testid="cell-actual"
                    title={t('budgets.page.budget.structure.element.action.show_transactions')}
                    className="text-xs underline-offset-2 hover:underline"
                    onClick={(e) => {
                      e.stopPropagation()
                      ctx.select(rk, i, e)
                      ctx.openTransactions(el, m)
                    }}
                  >
                    <Dash />
                  </button>
                ) : actual === null ? (
                  view.dash ? (
                    <span data-testid="cell-no-actual" className="text-xs">
                      <Dash />
                    </span>
                  ) : null
                ) : !view.balance || m <= ctx.selected || movesMoney(cell) ? (
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
                  <span className="ml-auto w-[58%] min-w-16 shrink-0">
                    <PlanCellInput
                      initial={editing.initial}
                      label={t('budgets.page.plan.cell.edit_aria', { name: displayName, month: ctx.monthLabel(m) })}
                      onCommit={ctx.finishEdit}
                      onCancel={ctx.cancelEdit}
                    />
                  </span>
                ) : (
                  <span data-testid="cell-planned" className="ml-auto shrink-0 text-[15px] whitespace-nowrap tabular-nums">
                    {view.plan !== null ? fmt(view.plan) : view.balance ? <Dash /> : ''}
                  </span>
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
              previewDisabled={ctx.commentsOpen || ctx.editMode || !!editing || ctx.dragSelect.active}
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
