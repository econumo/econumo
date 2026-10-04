import type { ReactNode } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useBudgetPeriodStore } from './budgetStore'
import { EMPTY_CELL, FIRST_COL, FOLD_LINE, FOLDER_INDENT, LINE, NAME_COL, SECOND_COL, THIRD_COL, foldOnLineClick } from './monthLayout'

/* edit mode appends a w-8 actions button to element rows; every line without
   one must pad the slot or its amount columns drift out of alignment */
export function ActionsSpacer() {
  return <span data-testid="actions-spacer" className="w-8 shrink-0" />
}

/** a column with no value at all: always muted, whatever colour its cell has */
export function Dash() {
  return <span className="text-muted-foreground/50">{EMPTY_CELL}</span>
}

/** a row whose currency is not the budget's names it once, next to the name */
export function CurrencyTag({ code }: { code: string }) {
  return (
    <span data-testid="currency-tag" className="shrink-0 rounded bg-muted px-1 text-[10px] font-medium text-muted-foreground">
      {code}
    </span>
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
}: {
  foldKey: string
  label: string
  headings: [string, string, string]
  sums: [ReactNode, ReactNode, ReactNode]
  actionsColumn: boolean
  testId: string
}) {
  const { t } = useTranslation()
  const folded = useBudgetPeriodStore((s) => !!s.planFolds[foldKey])
  const toggle = useBudgetPeriodStore((s) => s.togglePlanFold)
  const Chevron = folded ? ChevronRight : ChevronDown
  const cells = folded ? sums : headings
  return (
    <div
      className={`${LINE} ${FOLD_LINE} min-h-10 ${folded ? 'text-sm text-muted-foreground' : 'text-[10.5px] uppercase tracking-wider text-muted-foreground'}`}
      data-testid={testId}
      onClick={foldOnLineClick(() => toggle(foldKey))}
    >
      <button
        type="button"
        data-fold=""
        aria-expanded={!folded}
        title={t(folded ? 'common.button.expand.label' : 'common.button.collapse.label')}
        className={`${NAME_COL} py-1 text-left text-[15px] normal-case tracking-normal text-foreground`}
      >
        <Chevron aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
        <span className="truncate">{label}</span>
      </button>
      <span className={FIRST_COL}>{cells[0]}</span>
      <span className={SECOND_COL}>{cells[1]}</span>
      <span className={THIRD_COL}>{cells[2]}</span>
      {actionsColumn ? <ActionsSpacer /> : null}
    </div>
  )
}

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
}: {
  name: string
  folded: boolean
  onToggle: () => void
  /** null for a line without figures (an empty folder in edit mode) */
  sums: [ReactNode, ReactNode, ReactNode] | null
  /** the drag grip, before the name (edit mode) */
  handle?: ReactNode
  /** folder actions, right after the name (edit mode) */
  actions?: ReactNode
  actionsColumn: boolean
}) {
  const { t } = useTranslation()
  const Chevron = folded ? ChevronRight : ChevronDown
  return (
    <header className={`${LINE} ${FOLD_LINE} ${FOLDER_INDENT} min-h-9 text-sm text-muted-foreground`} onClick={foldOnLineClick(onToggle)}>
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
      {sums ? (
        <span data-testid="stat-line" className="contents">
          <span className={FIRST_COL}>{sums[0]}</span>
          <span className={SECOND_COL}>{sums[1]}</span>
          <span className={THIRD_COL}>{sums[2]}</span>
        </span>
      ) : null}
      {actionsColumn ? <ActionsSpacer /> : null}
    </header>
  )
}
