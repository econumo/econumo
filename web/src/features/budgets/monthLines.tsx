import type { HTMLAttributes, ReactNode } from 'react'
import { ChevronDown, ChevronRight, MoreVertical } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { useBudgetPeriodStore } from './budgetStore'
import type { MenuAction } from './monthLayout'
import {
  EMPTY_CELL,
  FIRST_COL,
  FOLD_LINE,
  FOLDER_INDENT,
  LINE,
  NAME_COL,
  PLAN_CROSSHAIR,
  PLAN_FIGURE_COL,
  PLAN_LINE,
  PLAN_NAME_COL,
  PLAN_SELECTED_TINT,
  SECOND_COL,
  THIRD_COL,
  foldOnLineClick,
  lineControlClass,
  useLineControls,
  useLineLayout,
} from './monthLayout'

/* edit mode appends a w-8 actions button to element rows; every line without
   one must pad the slot or its amount columns drift out of alignment */
export function ActionsSpacer() {
  return <span data-testid="actions-spacer" className="w-8 shrink-0" />
}

/** A line's figures in the current layout's columns: the Budget view's three fixed
 *  ones, or one per Plan month with the selected month tinted. */
export function FigureCells({ cells, cellClassName }: { cells: ReactNode[]; cellClassName?: (i: number) => string }) {
  const layout = useLineLayout()
  if (layout.kind === 'budget') {
    return (
      <>
        <span className={FIRST_COL}>{cells[0]}</span>
        <span className={SECOND_COL}>{cells[1]}</span>
        <span className={THIRD_COL}>{cells[2]}</span>
      </>
    )
  }
  return (
    <>
      {cells.map((cell, i) => (
        <span
          key={i}
          data-col={i}
          data-crosshair={i === layout.crosshairCol ? 'col' : undefined}
          className={[
            PLAN_FIGURE_COL,
            i === layout.selectedCol ? PLAN_SELECTED_TINT : '',
            i === layout.crosshairCol ? PLAN_CROSSHAIR : '',
            cellClassName?.(i) ?? '',
          ]
            .filter(Boolean)
            .join(' ')}
        >
          {cell}
        </span>
      ))}
    </>
  )
}

/** a column with no value at all: always muted, whatever colour its cell has */
export function Dash() {
  return <span className="text-muted-foreground/50">{EMPTY_CELL}</span>
}

/** A line's ⋮ menu: with a mouse it shows while the pointer is over the line (and
 *  stays while open); in a touch screen's edit mode it shows on every line. */
export function RowMenu({ name, actions }: { name: string; actions: MenuAction[] | undefined }) {
  const controls = useLineControls()
  const plan = useLineLayout().kind === 'plan'
  if (!actions || actions.length === 0) {
    return null
  }
  const menu = (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={`menu ${name}`}
          className={`size-7 shrink-0 text-muted-foreground ${lineControlClass(controls, 'line')}`}
        >
          <MoreVertical className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-44">
        {actions.map((a) => (
          <DropdownMenuItem
            key={a.label}
            className="whitespace-nowrap"
            variant={a.destructive ? 'destructive' : 'default'}
            disabled={a.disabled}
            onSelect={a.onSelect}
          >
            {a.label}
            {a.disabled && a.reason ? <span className="-ml-1 text-muted-foreground"> ({a.reason})</span> : null}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
  if (!plan || controls === 'always') {
    return menu
  }
  // The Plan grid's name column is fixed-width: a hidden ⋮ takes none of it, so a
  // long name only gives way while the line is hovered, the menu open or the button
  // focused.
  return (
    <span className="-ml-2 flex w-0 shrink-0 justify-end group-hover/line:ml-0 group-hover/line:w-7 has-[[data-state=open]]:ml-0 has-[[data-state=open]]:w-7 has-[:focus-visible]:ml-0 has-[:focus-visible]:w-7">
      {menu}
    </span>
  )
}

/** a row whose currency is not the budget's names it once, next to the name */
export function CurrencyTag({ code }: { code: string }) {
  return (
    <span data-testid="currency-tag" className="shrink-0 rounded bg-muted px-1 text-[10px] font-medium text-muted-foreground">
      {code}
    </span>
  )
}

/** The Plan grid's Σ: an open line's sums show only once the user asks for them —
 *  the rows right below already carry the figures. Shown on hover; once pressed it
 *  stays visible so the line says why it has numbers. */
function SumToggle({ sumKey, name }: { sumKey: string; name: string }) {
  const { t } = useTranslation()
  const shown = useBudgetPeriodStore((s) => !!s.planSumsShown[sumKey])
  const toggle = useBudgetPeriodStore((s) => s.togglePlanSums)
  const controls = useLineControls()
  return (
    <button
      type="button"
      aria-pressed={shown}
      aria-label={t(shown ? 'budgets.page.plan.sums.hide' : 'budgets.page.plan.sums.show', { name })}
      title={t(shown ? 'budgets.page.plan.sums.hide' : 'budgets.page.plan.sums.show', { name })}
      className={`flex size-6 shrink-0 items-center justify-center rounded text-sm hover:bg-accent ${shown ? 'text-foreground' : `text-muted-foreground ${lineControlClass(controls, 'line')}`}`}
      onClick={() => toggle(sumKey)}
    >
      Σ
    </button>
  )
}

/** One month's sum on an open Plan line: a Σ that shows on hovering the cell, and
 *  once pressed the sum itself, which hides again on a second press. Remembered per
 *  line and month, like the line's own Σ. */
function CellSum({ sumKey, index, name, value }: { sumKey: string; index: number; name: string; value: ReactNode }) {
  const { t } = useTranslation()
  const layout = useLineLayout()
  const controls = useLineControls()
  const month = layout.kind === 'plan' ? layout.months?.[index] : undefined
  const key = `${sumKey}@${month}`
  const shown = useBudgetPeriodStore((s) => !!s.planSumsShown[key])
  const toggle = useBudgetPeriodStore((s) => s.togglePlanSums)
  if (!month || layout.kind !== 'plan') {
    return null
  }
  const label = t(shown ? 'budgets.page.plan.sums.hide_month' : 'budgets.page.plan.sums.show_month', { name, month: layout.monthLabels?.[index] ?? month })
  return (
    <button
      type="button"
      aria-pressed={shown}
      aria-label={label}
      title={label}
      className="group/sum -mx-2 flex h-full min-w-0 flex-1 items-center justify-end gap-1 self-stretch px-2"
      onClick={() => toggle(key)}
    >
      {shown ? (
        value
      ) : (
        <span
          aria-hidden="true"
          className={`text-sm text-muted-foreground ${controls === 'always' ? '' : 'opacity-0 group-hover/sum:opacity-100 group-focus-visible/sum:opacity-100'}`}
        >
          Σ
        </span>
      )}
    </button>
  )
}

/** A section's line: open, it names the section's columns; folded, it carries the
 *  section's sums in those columns. The fold state is the one the Plan grid uses. */
export function MonthSectionHeader({
  foldKey,
  label,
  headings,
  sums,
  actionsColumn,
  testId,
  menu,
  sumsOnDemand = false,
}: {
  foldKey: string
  label: string
  headings: ReactNode[]
  sums: ReactNode[]
  actionsColumn: boolean
  testId: string
  /** the section's ⋮ menu (create folder, choose savings accounts) */
  menu?: MenuAction[]
  /** Plan grid: an open line's sums wait for its Σ */
  sumsOnDemand?: boolean
}) {
  const { t } = useTranslation()
  const folded = useBudgetPeriodStore((s) => !!s.planFolds[foldKey])
  const sumsAsked = useBudgetPeriodStore((s) => !!s.planSumsShown[foldKey])
  const toggle = useBudgetPeriodStore((s) => s.togglePlanFold)
  const Chevron = folded ? ChevronRight : ChevronDown
  const layout = useLineLayout()
  const plan = layout.kind === 'plan'
  const showSums = folded || plan
  const blank = plan && sumsOnDemand && !folded && !sumsAsked
  const foldButton = (
    <button
      type="button"
      data-fold=""
      aria-expanded={!folded}
      title={t(folded ? 'common.button.expand.label' : 'common.button.collapse.label')}
      className={`${plan ? 'flex min-w-0 flex-1 items-center gap-2' : NAME_COL} py-1 text-left text-[15px] normal-case tracking-normal text-foreground`}
    >
      <Chevron aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
      <span className="truncate">{label}</span>
    </button>
  )
  return (
    <div
      className={`${plan ? PLAN_LINE : LINE} ${FOLD_LINE} min-h-10 ${showSums ? 'text-sm text-muted-foreground' : 'text-[10.5px] uppercase tracking-wider text-muted-foreground'}`}
      data-testid={testId}
      onClick={foldOnLineClick(() => toggle(foldKey))}
    >
      {plan ? (
        <span className={PLAN_NAME_COL}>
          {foldButton}
          {sumsOnDemand && !folded ? <SumToggle sumKey={foldKey} name={label} /> : null}
          <RowMenu name={label} actions={menu} />
        </span>
      ) : (
        <>
          {foldButton}
          <RowMenu name={label} actions={menu} />
        </>
      )}
      <FigureCells
        cells={blank ? sums.map((value, i) => <CellSum key={i} sumKey={foldKey} index={i} name={label} value={value} />) : showSums ? sums : headings}
      />
      {actionsColumn ? <ActionsSpacer /> : null}
    </div>
  )
}

const blankCells = (layout: ReturnType<typeof useLineLayout>): null[] => Array.from({ length: layout.kind === 'plan' ? layout.cols : 3 }, () => null)

/** A folder's line inside a section: the whole line folds the folder's rows; its
 *  sums stay in the row columns either way. */
export function FolderLine({
  name,
  folded,
  onToggle,
  sums,
  handle,
  actions,
  actionsColumn,
  menu,
  nameCell,
  sumKey,
}: {
  name: string
  folded: boolean
  onToggle: () => void
  /** null for a folder with nothing in it: each column reads as a dash */
  sums: ReactNode[] | null
  /** the drag grip, before the name (edit mode) */
  handle?: ReactNode
  /** folder actions, right after the name (edit mode) */
  actions?: ReactNode
  actionsColumn: boolean
  /** the folder's ⋮ menu, shown on hover */
  menu?: MenuAction[]
  /** the Plan grid's name column: a gridcell the keyboard selection can land on */
  nameCell?: HTMLAttributes<HTMLSpanElement>
  /** Plan grid: the folder's fold key; an open folder's sums wait for its Σ */
  sumKey?: string
}) {
  const { t } = useTranslation()
  const layout = useLineLayout()
  const plan = layout.kind === 'plan'
  const sumsAsked = useBudgetPeriodStore((s) => (sumKey ? !!s.planSumsShown[sumKey] : false))
  const onDemand = plan && sumKey !== undefined && sums !== null && !folded
  const onDemandCells = onDemand && !sumsAsked
  // an open folder with nothing in it has nothing to sum: no Σ and no dashes either
  const blank = plan && sumKey !== undefined && sums === null && !folded
  const Chevron = folded ? ChevronRight : ChevronDown
  const nameParts = (
    <>
      {handle}
      <button
        type="button"
        data-fold=""
        aria-expanded={!folded}
        title={t(folded ? 'common.button.expand.label' : 'common.button.collapse.label')}
        className="flex min-w-0 items-center gap-1.5 py-1 text-left"
      >
        <Chevron aria-hidden="true" className="size-3.5 shrink-0" />
        <span className="truncate" title={name}>
          {name}
        </span>
      </button>
      {actions}
      <span className="flex-1" />
      {onDemand ? <SumToggle sumKey={sumKey} name={name} /> : null}
      <RowMenu name={name} actions={menu} />
    </>
  )
  const dashes = Array.from({ length: plan ? layout.cols : 3 }, (_, i) => <Dash key={i} />)
  return (
    <header className={`${plan ? PLAN_LINE : LINE} ${FOLD_LINE} ${plan ? '' : FOLDER_INDENT} relative min-h-9 text-sm text-muted-foreground`} onClick={foldOnLineClick(onToggle)}>
      {plan ? (
        <span {...nameCell} className={`${PLAN_NAME_COL} ${FOLDER_INDENT} ${nameCell?.className ?? ''}`.trim()}>
          {nameParts}
        </span>
      ) : (
        nameParts
      )}
      {/* an empty folder reads as dashes in the same columns, so its ⋮ lines up
          with the others' */}
      <span data-testid={sums ? 'stat-line' : 'empty-folder-sums'} className="contents">
        <FigureCells
          cells={
            onDemandCells && sums && sumKey !== undefined
              ? sums.map((value, i) => <CellSum key={i} sumKey={sumKey} index={i} name={name} value={value} />)
              : blank
                ? blankCells(layout)
                : (sums ?? dashes)
          }
        />
      </span>
      {actionsColumn ? <ActionsSpacer /> : null}
    </header>
  )
}

/** A totals line: its label in the name column, one value per column. The Budget
 *  view has a single value, in the last column. */
export function TotalLine({
  testId,
  label,
  values,
  strong = false,
  negative = false,
  actionsColumn,
}: {
  testId: string
  label: string
  values: ReactNode[]
  /** the line the block ends on: full-colour label */
  strong?: boolean
  negative?: boolean | boolean[]
  actionsColumn: boolean
}) {
  const plan = useLineLayout().kind === 'plan'
  const isNegative = (i: number) => (Array.isArray(negative) ? !!negative[i] : negative)
  return (
    <div className={plan ? `${PLAN_LINE} min-h-8` : `${LINE} min-h-8 py-0.5`} data-testid={testId}>
      <span className={`${plan ? PLAN_NAME_COL : NAME_COL} text-sm ${strong ? '' : 'text-muted-foreground'}`}>
        <span className="truncate">{label}</span>
      </span>
      {plan ? (
        <FigureCells cells={values} cellClassName={(i) => `text-[15px] ${isNegative(i) ? 'text-expense' : ''}`.trim()} />
      ) : (
        <span className={`${THIRD_COL} text-[15px] ${isNegative(0) ? 'text-expense' : ''}`}>{values[0]}</span>
      )}
      {actionsColumn ? <ActionsSpacer /> : null}
    </div>
  )
}
