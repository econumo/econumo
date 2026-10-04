import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { DndContext, DragOverlay, MeasuringStrategy, PointerSensor, useSensor, useSensors } from '@dnd-kit/core'
import type { DragEndEvent, DragOverEvent, DragStartEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { snapRowToPointer } from '@/lib/dnd'
import { afterIdFromDrop } from '@/lib/ordering'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { EntityIcon } from '@/components/EntityIcon'
import { cmp, isZero } from '@/lib/decimal'
import { moneyFormat } from '@/lib/money'
import type { MoneyFormatOptions } from '@/lib/money'
import type { BudgetDto, BudgetSavingsElementDto } from '@/api/dto/budget'
import { BudgetElementType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { elementDisplayName, makeBudgetExchange, totalsWithSavings } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'
import type { BudgetTransactionsTarget } from './BudgetTransactionsDialog'
import { ActionsSpacer, CurrencyTag, Dash, FolderLine, MonthSectionHeader, RowMenu } from './monthLines'
import type { MenuAction } from './monthLayout'
import { CHILD_INDENT, FIRST_COL, LINE, NAME_COL, ROW_INDENT, SECOND_COL, THIRD_COL } from './monthLayout'
import { leftToReceive } from './phoneMonth'
import type { IncomeGroup, PlanCellFigures, PlanMonthFigures, SheetTarget } from './phoneMonth'
import { arrangementItem, dropIndicatorFor, ENVELOPE_DROP, envelopeCollisions, moveElementInArrangement, placeFromEnvelope, preferRowCollisions } from './elementMove'
import type { DropIndicator, ElementContainer, ElementMoveItem } from './elementMove'
import { DragChild, DragFolder, DragGhost, DragRow, EnvelopeDrop, FolderGrip } from './MonthDrag'
import { isEnvelopeType } from './elementEdit'

export type FlowTarget = Extract<SheetTarget, { kind: 'plan' } | { kind: 'savings' }>

const cellOpts = (currency: CurrencyDto | undefined): MoneyFormatOptions => ({
  showCurrency: false,
  useNativePrecision: false,
  maxPrecision: currency?.fractionDigits ?? 2,
})

function FlowRow({
  testId,
  icon,
  name,
  tag,
  muted = false,
  planned,
  actual,
  third,
  actionsColumn,
  toggle,
  menu,
}: {
  testId: string
  icon: string
  name: string
  /** the row's currency code, when it is not the budget's */
  tag?: string
  muted?: boolean
  planned: ReactNode
  actual: ReactNode
  third: ReactNode
  actionsColumn: boolean
  /** an envelope's name folds and unfolds its categories */
  toggle?: { open: boolean; onToggle: () => void }
  menu?: MenuAction[]
}) {
  const { t } = useTranslation()
  const Chevron = toggle?.open ? ChevronDown : ChevronRight
  const label = (
    <>
      {toggle ? <Chevron className="size-3.5 shrink-0 text-muted-foreground" /> : <span className="w-3.5 shrink-0" />}
      <EntityIcon name={icon} className="text-lg text-muted-foreground" />
      <span className={`min-w-0 truncate text-[15px] ${muted ? 'text-muted-foreground' : ''}`} title={name}>
        {name}
      </span>
      {tag ? <CurrencyTag code={tag} /> : null}
    </>
  )
  return (
    <div className={`${LINE} ${ROW_INDENT} min-h-10 rounded-md py-1.5 hover:bg-accent/50`} data-testid={testId}>
      {toggle ? (
        <button
          type="button"
          aria-expanded={toggle.open}
          title={t(toggle.open ? 'common.button.collapse.label' : 'common.button.expand.label')}
          onClick={toggle.onToggle}
          className={`${NAME_COL} text-left`}
        >
          {label}
        </button>
      ) : (
        <span className={NAME_COL}>{label}</span>
      )}
      <RowMenu name={name} actions={menu} />
      <span className={`${FIRST_COL} text-[15px]`} data-testid="flow-planned">
        {planned}
      </span>
      <span className={`${SECOND_COL} text-[15px] text-muted-foreground`} data-testid="flow-actual">
        {actual}
      </span>
      <span className={`${THIRD_COL} text-[15px]`} data-testid="flow-third">
        {third}
      </span>
      {actionsColumn ? <ActionsSpacer /> : null}
    </div>
  )
}

interface MonthFlowsProps {
  budget: BudgetDto
  currencies: CurrencyDto[]
  /** null while get-budget-plan loads: the Income section waits for it */
  planMonth: PlanMonthFigures | null
  future: boolean
  actionsColumn: boolean
  /** the planned cell: the page decides between the inline editor, the item sheet and comments */
  renderPlanned: (target: FlowTarget, text: string) => ReactNode
  onShowTransactions?: (target: BudgetTransactionsTarget) => void
  /** ⋮ menus, shown on hover */
  incomeMenu?: (cell: PlanCellFigures) => MenuAction[] | undefined
  incomeFolderMenu?: (group: IncomeGroup) => MenuAction[] | undefined
  savingsMenu?: (row: BudgetSavingsElementDto) => MenuAction[] | undefined
  incomeSectionMenu?: MenuAction[]
  savingsSectionMenu?: MenuAction[]
  /** drag and drop on hover, for anyone who may configure the budget */
  drag?: {
    /** onFailed: the move did not happen (a hidden row shows again) */
    onMoveIncome: (item: ElementMoveItem, onFailed?: () => void) => void
    /** an income category goes into an income envelope */
    onMoveIncomeIntoEnvelope: (id: Id, envelopeId: Id, onFailed: () => void) => void
    onMoveIncomeFolder: (folderId: Id, afterId: Id | null) => void
    onMoveSavings: (id: Id, afterId: Id | null) => void
  }
}

/** Income and Savings for one month, above the expenses table, in the table's columns:
 *  Planned · Received · To receive, and Planned · Saved · Balance. */
export function MonthFlows({
  budget,
  currencies,
  planMonth,
  future,
  actionsColumn,
  renderPlanned,
  onShowTransactions,
  incomeMenu,
  incomeFolderMenu,
  savingsMenu,
  incomeSectionMenu,
  savingsSectionMenu,
  drag,
}: MonthFlowsProps) {
  const { t } = useTranslation()
  const base = budget.meta.currencyId
  const currencyOf = (id: Id | null) => currencies.find((c) => c.id === (id ?? base))
  const fmt = (amount: string, currencyId: Id | null = base) => {
    const c = currencyOf(currencyId)
    return moneyFormat(amount, c, cellOpts(c))
  }
  const tagOf = (currencyId: Id | null) => (currencyId && currencyId !== base ? currencyOf(currencyId)?.code : undefined)
  const exchangeFn = makeBudgetExchange(budget, currencies)
  const unfolded = useBudgetPeriodStore((s) => s.unfoldedElements)
  const toggleElement = useBudgetPeriodStore((s) => s.toggleElement)
  const planFolds = useBudgetPeriodStore((s) => s.planFolds)
  const togglePlanFold = useBudgetPeriodStore((s) => s.togglePlanFold)
  const incomeFolded = !!planFolds.income
  const savingsFolded = !!planFolds.savings
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))
  // a dropped order holds until the refetched figures replace it, so nothing snaps back
  const [incomePreview, setIncomePreview] = useState<{ containers: ElementContainer[]; folderIds: string[] } | null>(null)
  const [savingsPreview, setSavingsPreview] = useState<string[] | null>(null)
  const [incomeFolderDragging, setIncomeFolderDragging] = useState(false)
  // an income category on its way into or out of an envelope: hidden until the
  // refetched figures show it in its new place
  const [pendingMemberId, setPendingMemberId] = useState<string | null>(null)
  // the income row or category being dragged, and where it would land
  const [incomeDragId, setIncomeDragId] = useState<string | null>(null)
  const [incomeIndicator, setIncomeIndicator] = useState<DropIndicator | null>(null)
  useEffect(() => {
    setIncomePreview(null)
    setPendingMemberId(null)
  }, [planMonth])
  useEffect(() => {
    setSavingsPreview(null)
  }, [budget])

  const actualCell = (target: BudgetTransactionsTarget | null, amount: string, currencyId: Id | null) => {
    if (future) {
      return <Dash />
    }
    const text = fmt(amount, currencyId)
    if (!target || !onShowTransactions) {
      return text
    }
    return (
      <button
        type="button"
        title={t('budgets.page.budget.structure.element.action.show_transactions')}
        aria-label={`transactions ${target.name}`}
        className="underline-offset-2 hover:text-foreground hover:underline"
        onClick={() => onShowTransactions(target)}
      >
        {text}
      </button>
    )
  }

  const incomeRow = (row: PlanCellFigures, draggable = false) => {
    const el = row.element
    const name = elementDisplayName(el.id, el.name, t)
    // income Uncategorized gathers income booked in expense categories too: no list names it
    const listTarget = el.id === UNCATEGORIZED_ID ? null : { id: el.id, type: el.type, name, icon: el.icon, currencyId: el.currencyId }
    // an envelope unfolds even while empty: its list is where a category is dropped
    const envelope = isEnvelopeType(el.type)
    const children = el.children.filter((c) => c.id !== pendingMemberId)
    const expandable = children.length > 0 || envelope
    const open = expandable && !!unfolded[el.id]
    const childDrag = draggable && envelope && el.isArchived === 0
    const childList = (
      <div className="pb-1" data-testid={`month-income-children-${el.id}`}>
        {children.length === 0 ? (
          <p className={`${CHILD_INDENT} px-2 py-1 text-xs text-muted-foreground`}>{t('budgets.page.budget.structure.empty_envelope.note')}</p>
        ) : null}
        {children.map((child) => {
          const childName = elementDisplayName(child.id, child.name, t)
          const childLine = (
            <div
              key={child.id}
              className={`${LINE} ${CHILD_INDENT} min-h-8 rounded-md py-1 text-sm text-muted-foreground hover:bg-accent/50`}
              data-testid={`month-income-child-${child.id}`}
            >
              <span className={NAME_COL}>
                <EntityIcon name={child.icon} className="text-lg" />
                <span className="truncate" title={childName}>
                  {childName}
                </span>
              </span>
              <span className={FIRST_COL} />
              <span className={SECOND_COL}>
                {actualCell(
                  { id: child.id, type: child.type, name: childName, icon: child.icon, currencyId: el.currencyId, parent: { id: el.id, type: el.type } },
                  child.cells[planMonth?.index ?? 0]?.actual ?? '0',
                  el.currencyId,
                )}
              </span>
              <span className={THIRD_COL} />
              {actionsColumn ? <ActionsSpacer /> : null}
            </div>
          )
          return childDrag ? (
            <DragChild key={child.id} id={child.id}>
              {childLine}
            </DragChild>
          ) : (
            childLine
          )
        })}
      </div>
    )
    const line = (
      <div key={`${el.id}:${el.type}`}>
        <FlowRow
          testId={`month-income-row-${el.id}`}
          icon={el.icon}
          name={name}
          tag={tagOf(el.currencyId)}
          muted={el.isArchived === 1}
          planned={el.id === UNCATEGORIZED_ID ? <Dash /> : renderPlanned({ kind: 'plan', cell: row }, fmt(row.planned, el.currencyId))}
          actual={actualCell(listTarget, row.actual, el.currencyId)}
          third={
            // with no plan there is nothing to expect: a dash, not a zero
            el.id === UNCATEGORIZED_ID || isZero(row.planned) ? (
              <Dash />
            ) : (
              fmt(leftToReceive(row.planned, row.actual), el.currencyId)
            )
          }
          actionsColumn={actionsColumn}
          toggle={expandable ? { open, onToggle: () => toggleElement(el.id) } : undefined}
          menu={incomeMenu?.(row)}
        />
        {open ? (childDrag ? <EnvelopeDrop envelopeId={el.id}>{childList}</EnvelopeDrop> : childList) : null}
      </div>
    )
    return draggable ? (
      <DragRow
        key={`${el.id}:${el.type}`}
        id={el.id}
        indicator={incomeIndicator?.kind === 'row' && incomeIndicator.id === el.id ? incomeIndicator.edge : undefined}
      >
        {line}
      </DragRow>
    ) : (
      line
    )
  }

  // the expense table's labels: the folder-less rows are named only when there are
  // folders, Uncategorized stands alone, archived rows sit under Archived
  const groupName = (g: IncomeGroup, groups: IncomeGroup[]): string | null => {
    switch (g.kind) {
      case 'folder':
        return g.name
      case 'loose':
        return groups.some((o) => o.kind === 'folder') ? t('budgets.page.plan.menu.no_folder') : null
      case 'uncategorized':
        return null
      case 'archived':
        return t('budgets.page.budget.structure.in_archive')
    }
  }
  const incomeGroup = (g: IncomeGroup, groups: IncomeGroup[]) => {
    const name = groupName(g, groups)
    // the phone's keys: a folder's own id, and income's own No folder and Archived
    const foldKey = g.kind === 'folder' ? g.id : `__income${g.id}`
    const folded = name !== null && !!planFolds[foldKey]
    // folders and the folder-less rows take part in drag and drop; Uncategorized and
    // archived rows stay where they are
    const draggable = !!drag && (g.kind === 'folder' || g.kind === 'loose')
    const section = (
      <div key={g.id} className="pt-1" data-testid={`month-income-folder-${g.id}`}>
        {name !== null ? (
          <FolderLine
            handle={draggable && g.kind === 'folder' ? <FolderGrip name={name} /> : undefined}
            name={name}
            folded={folded}
            onToggle={() => togglePlanFold(foldKey)}
            menu={incomeFolderMenu?.(g)}
            sums={[fmt(g.planned), future ? <Dash key="dash-235-1" /> : fmt(g.received), isZero(g.planned) ? <Dash key="dash-235-2" /> : fmt(g.toReceive)]}
            actionsColumn={actionsColumn}
          />
        ) : null}
        {folded || incomeFolderDragging ? null : g.rows.length === 0 ? (
          <p className={`${ROW_INDENT} px-2 py-1 text-xs text-muted-foreground`}>{t('budgets.page.budget.structure.empty_folder.note')}</p>
        ) : (
          g.rows.map((row) => incomeRow(row, draggable))
        )}
      </div>
    )
    return draggable ? (
      <DragFolder
        key={g.id}
        sortableId={g.kind === 'folder' ? g.id : null}
        dropId={`bfolder:${g.kind === 'folder' ? g.id : 'null'}`}
        rowIds={g.rows.map((r) => r.element.id)}
        indicator={incomeIndicator?.kind === 'folder' && (incomeIndicator.folderId ?? 'null') === (g.kind === 'folder' ? g.id : 'null')}
        folderDragging={incomeFolderDragging}
      >
        {section}
      </DragFolder>
    ) : (
      section
    )
  }

  const savingsRow = (row: BudgetSavingsElementDto) => (
    <FlowRow
      key={row.id}
      testId={`month-savings-row-${row.id}`}
      icon={row.icon}
      name={row.name}
      tag={tagOf(row.currencyId)}
      muted={row.isArchived === 1}
      planned={renderPlanned({ kind: 'savings', row }, fmt(row.budgeted, row.currencyId))}
      actual={actualCell({ id: row.id, type: BudgetElementType.SAVINGS, name: row.name, icon: row.icon, currencyId: row.currencyId }, row.spent, row.currencyId)}
      third={<span title={t('budgets.page.savings.balance_hint')}>{row.closingBalance !== undefined ? fmt(row.closingBalance, row.currencyId) : <Dash />}</span>}
      actionsColumn={actionsColumn}
      menu={savingsMenu?.(row)}
    />
  )

  const incomeGroups: IncomeGroup[] = (planMonth?.income.groups ?? []).map((g) =>
    pendingMemberId && g.rows.some((r) => r.element.id === pendingMemberId) ? { ...g, rows: g.rows.filter((r) => r.element.id !== pendingMemberId) } : g,
  )
  // the income envelope each income category sits in; a category may go into any
  // other live income envelope
  const envelopeOfCategory = new Map(
    incomeGroups.flatMap((g) => g.rows).flatMap((r) => (isEnvelopeType(r.element.type) ? r.element.children.map((c) => [c.id, r.element.id] as const) : [])),
  )
  const isIncomeCategory = (id: string) =>
    envelopeOfCategory.has(id) || incomeGroups.some((g) => g.rows.some((r) => r.element.id === id && r.element.type === BudgetElementType.INCOME_CATEGORY))
  const canEnterEnvelope = (activeId: string, envelopeId: string) => isIncomeCategory(activeId) && envelopeOfCategory.get(activeId) !== envelopeId

  // the income groups as a drop arrangement: each folder, then the folder-less rows
  const arrangementOf = (groups: IncomeGroup[]): ElementContainer[] => [
    ...groups.filter((g) => g.kind === 'folder').map((g) => ({ folderId: g.id as Id | null, ids: g.rows.map((r) => r.element.id) })),
    { folderId: null, ids: groups.find((g) => g.kind === 'loose')?.rows.map((r) => r.element.id) ?? [] },
  ]
  const shownIncomeGroups: IncomeGroup[] = (() => {
    if (!incomePreview) {
      return incomeGroups
    }
    const rowById = new Map(incomeGroups.flatMap((g) => g.rows).map((r) => [r.element.id, r]))
    const rowsOf = (folderId: Id | null) =>
      (incomePreview.containers.find((c) => c.folderId === folderId)?.ids ?? []).flatMap((id) => {
        const row = rowById.get(id)
        return row ? [row] : []
      })
    const folders = incomePreview.folderIds.flatMap((id) => {
      const g = incomeGroups.find((x) => x.kind === 'folder' && x.id === id)
      return g ? [{ ...g, rows: rowsOf(g.id) }] : []
    })
    const loose = incomeGroups.find((g) => g.kind === 'loose') ?? { kind: 'loose' as const, id: '__no_folder__', name: null, rows: [], planned: '0', received: '0', toReceive: '0' }
    const looseRows = rowsOf(null)
    const rest = incomeGroups.filter((g) => g.kind !== 'folder' && g.kind !== 'loose')
    return [...folders, ...(looseRows.length > 0 ? [{ ...loose, rows: looseRows }] : []), ...rest]
  })()
  const incomeFolderIds = shownIncomeGroups.filter((g) => g.kind === 'folder').map((g) => g.id)

  const onIncomeDragStart = ({ active }: DragStartEvent) => {
    if (incomeFolderIds.includes(String(active.id))) {
      setIncomeFolderDragging(true)
      return
    }
    setIncomeDragId(String(active.id))
  }
  const onIncomeDragOver = ({ active, over }: DragOverEvent) => {
    const activeId = String(active.id)
    const overId = over ? String(over.id) : null
    const fromEnvelope = envelopeOfCategory.get(activeId)
    if (incomeFolderIds.includes(activeId) || !overId || overId === activeId || overId === fromEnvelope) {
      setIncomeIndicator(null)
      return
    }
    setIncomeIndicator(
      dropIndicatorFor(arrangementOf(shownIncomeGroups), activeId, overId, {
        fromEnvelope: fromEnvelope !== undefined,
        isFolded: (folderId) => !!planFolds[folderId ?? '__income__no_folder__'],
      }),
    )
  }
  const draggedIncome = (() => {
    for (const row of shownIncomeGroups.flatMap((g) => g.rows)) {
      if (row.element.id === incomeDragId) {
        return { icon: row.element.icon, name: elementDisplayName(row.element.id, row.element.name, t) }
      }
      const child = row.element.children.find((c) => c.id === incomeDragId)
      if (child) {
        return { icon: child.icon, name: child.name }
      }
    }
    return null
  })()
  const onIncomeDragEnd = ({ active, over }: DragEndEvent) => {
    setIncomeFolderDragging(false)
    setIncomeDragId(null)
    setIncomeIndicator(null)
    if (!drag || !over || active.id === over.id) {
      return
    }
    const activeId = String(active.id)
    const overId = String(over.id)
    const base = arrangementOf(shownIncomeGroups)
    if (overId.startsWith(ENVELOPE_DROP)) {
      const envelopeId = overId.slice(ENVELOPE_DROP.length)
      if (canEnterEnvelope(activeId, envelopeId)) {
        setPendingMemberId(activeId)
        drag.onMoveIncomeIntoEnvelope(activeId, envelopeId, () => setPendingMemberId(null))
      }
      return
    }
    const fromEnvelope = envelopeOfCategory.get(activeId)
    if (fromEnvelope) {
      // dropped back on its own envelope: it stays where it is
      const item = overId === fromEnvelope ? null : placeFromEnvelope(base, activeId, overId)
      if (item) {
        setPendingMemberId(activeId)
        drag.onMoveIncome(item, () => setPendingMemberId(null))
      }
      return
    }
    if (incomeFolderIds.includes(activeId)) {
      const from = incomeFolderIds.indexOf(activeId)
      const to = incomeFolderIds.indexOf(overId.replace(/^bfolder:/, ''))
      if (to === -1 || from === to) {
        return
      }
      const reordered = arrayMove(incomeFolderIds, from, to)
      setIncomePreview({ containers: base, folderIds: reordered })
      drag.onMoveIncomeFolder(activeId, afterIdFromDrop(reordered, activeId))
      return
    }
    const moved = moveElementInArrangement(base, activeId, overId)
    const item = arrangementItem(moved, activeId)
    const before = arrangementItem(base, activeId)
    if (moved === base || !item || (before && before.folderId === item.folderId && before.position === item.position)) {
      return
    }
    setIncomePreview({ containers: moved, folderIds: incomeFolderIds })
    drag.onMoveIncome(item)
  }

  const savingsRows = [...(budget.structure.savings ?? [])].sort((a, b) => a.isArchived - b.isArchived || a.position - b.position)
  // live savings rows reorder among themselves only
  const liveSavings = (() => {
    const live = savingsRows.filter((r) => r.isArchived === 0)
    if (!savingsPreview) {
      return live
    }
    return savingsPreview.flatMap((id) => {
      const row = live.find((r) => r.id === id)
      return row ? [row] : []
    })
  })()
  const onSavingsDragEnd = ({ active, over }: DragEndEvent) => {
    const ids = liveSavings.map((r) => r.id)
    const activeId = String(active.id)
    const overId = over ? String(over.id) : null
    if (!drag || !overId || overId === activeId || !ids.includes(overId) || !ids.includes(activeId)) {
      return
    }
    const reordered = arrayMove(ids, ids.indexOf(activeId), ids.indexOf(overId))
    setSavingsPreview(reordered)
    drag.onMoveSavings(activeId, afterIdFromDrop(reordered, activeId))
  }
  const savingsSum = totalsWithSavings({ budgeted: '0', spent: '0', available: '0', carry: '0' }, budget, exchangeFn)

  return (
    <>
      {/* the budget currency is named once, above every figure on the page */}
      <div className={`${LINE} text-[10.5px] uppercase tracking-wider text-muted-foreground`} data-testid="month-currency">
        {currencyOf(base)?.code}
      </div>
      {planMonth ? (
        <section className="border-t pt-1 pb-1" data-testid="month-income">
          <MonthSectionHeader
            foldKey="income"
            testId="month-income-header"
            label={t('budgets.page.plan.section.income')}
            headings={[t('budgets.page.savings.planned'), t('budgets.page.sheet.received'), t('budgets.page.budget.structure.tab.to_receive')]}
            sums={[
              fmt(planMonth.income.planned),
              future ? <Dash key="dash-277-1" /> : fmt(planMonth.income.received),
              isZero(planMonth.income.planned) ? <Dash key="dash-278-1" /> : fmt(planMonth.income.toReceive),
            ]}
            actionsColumn={actionsColumn}
            menu={incomeSectionMenu}
          />
          {incomeFolded ? null : drag ? (
            <DndContext
              sensors={sensors}
              collisionDetection={envelopeCollisions(canEnterEnvelope)}
              measuring={{ droppable: { strategy: MeasuringStrategy.Always } }}
              modifiers={[snapRowToPointer]}
              onDragStart={onIncomeDragStart}
              onDragOver={onIncomeDragOver}
              onDragEnd={onIncomeDragEnd}
              onDragCancel={() => {
                setIncomeFolderDragging(false)
                setIncomeDragId(null)
                setIncomeIndicator(null)
              }}
            >
              <SortableContext items={incomeFolderIds} strategy={verticalListSortingStrategy}>
                {shownIncomeGroups.map((g) => incomeGroup(g, shownIncomeGroups))}
              </SortableContext>
              {/* no drop animation: the moved row shows in its new place instead */}
              <DragOverlay dropAnimation={null}>{draggedIncome ? <DragGhost icon={draggedIncome.icon} name={draggedIncome.name} /> : null}</DragOverlay>
            </DndContext>
          ) : (
            incomeGroups.map((g) => incomeGroup(g, incomeGroups))
          )}
        </section>
      ) : null}
      {savingsRows.length > 0 ? (
        <section className="border-t pt-1 pb-1" data-testid="month-savings">
          <MonthSectionHeader
            foldKey="savings"
            testId="month-savings-header"
            label={t('budgets.page.plan.section.savings')}
            headings={[t('budgets.page.savings.planned'), t('budgets.page.savings.saved'), t('budgets.page.sheet.balance')]}
            sums={[
              fmt(savingsSum.budgeted),
              future ? <Dash key="dash-294-1" /> : fmt(savingsSum.spent),
              planMonth?.savingsBalance != null ? fmt(planMonth.savingsBalance) : <Dash key="dash-295-1" />,
            ]}
            actionsColumn={actionsColumn}
            menu={savingsSectionMenu}
          />
          {savingsFolded ? null : drag ? (
            <DndContext
              sensors={sensors}
              collisionDetection={preferRowCollisions}
              modifiers={[snapRowToPointer]}
              onDragEnd={onSavingsDragEnd}
            >
              <SortableContext items={liveSavings.map((r) => r.id)} strategy={verticalListSortingStrategy}>
                {liveSavings.map((row) => (
                  <DragRow key={row.id} id={row.id}>
                    {savingsRow(row)}
                  </DragRow>
                ))}
              </SortableContext>
              {/* a deleted account's row is read-only history: no grip */}
              {savingsRows.filter((r) => r.isArchived === 1).map(savingsRow)}
            </DndContext>
          ) : (
            savingsRows.map(savingsRow)
          )}
        </section>
      ) : null}
    </>
  )
}

function TotalLine({
  testId,
  label,
  value,
  strong = false,
  negative = false,
  actionsColumn,
}: {
  testId: string
  label: string
  value: ReactNode
  /** the line the block ends on: full-colour label */
  strong?: boolean
  negative?: boolean
  actionsColumn: boolean
}) {
  return (
    <div className={`${LINE} min-h-8 py-0.5`} data-testid={testId}>
      <span className={`${NAME_COL} text-sm ${strong ? '' : 'text-muted-foreground'}`}>
        <span className="truncate">{label}</span>
      </span>
      <span className={`${THIRD_COL} text-[15px] ${negative ? 'text-expense' : ''}`}>{value}</span>
      {actionsColumn ? <ActionsSpacer /> : null}
    </div>
  )
}

/** The phone totals card's lines under the Total row: what came in, went out and was
 *  set aside this month, then where the savings and the everyday money end up. */
export function MonthTotalsLines({
  budget,
  currencies,
  planMonth,
  expensesSpent,
  future,
  actionsColumn,
}: {
  budget: BudgetDto
  currencies: CurrencyDto[]
  planMonth: PlanMonthFigures | null
  expensesSpent: string
  future: boolean
  actionsColumn: boolean
}) {
  const { t } = useTranslation()
  const base = currencies.find((c) => c.id === budget.meta.currencyId)
  const fmt = (amount: string) => moneyFormat(amount, base, cellOpts(base))
  const hasSavings = (budget.structure.savings ?? []).length > 0
  const savingsSpent = hasSavings ? totalsWithSavings({ budgeted: '0', spent: '0', available: '0', carry: '0' }, budget, makeBudgetExchange(budget, currencies)).spent : null
  const shared = { actionsColumn }
  return (
    <div className="flex flex-col pt-1" data-testid="month-totals-lines">
      {planMonth ? (
        <TotalLine testId="month-total-income" label={t('budgets.page.plan.totals.income')} value={future ? <Dash /> : fmt(planMonth.income.received)} {...shared} />
      ) : null}
      <TotalLine testId="month-total-expenses" label={t('budgets.page.plan.totals.expenses')} value={future ? <Dash /> : fmt(expensesSpent)} {...shared} />
      {planMonth && !isZero(planMonth.transfersNet) ? (
        <TotalLine testId="month-total-transfers" label={t('budgets.page.plan.totals.transfers')} value={fmt(planMonth.transfersNet)} {...shared} />
      ) : null}
      {savingsSpent !== null ? (
        <TotalLine testId="month-total-savings" label={t('budgets.page.plan.totals.savings')} value={future ? <Dash /> : fmt(savingsSpent)} {...shared} />
      ) : null}
      {planMonth?.savingsBalance != null ? (
        <TotalLine testId="month-total-savings-balance" label={t('budgets.page.plan.totals.savings_balance')} value={fmt(planMonth.savingsBalance)} {...shared} />
      ) : null}
      {planMonth ? (
        <TotalLine
          testId="month-total-balance"
          label={t('budgets.page.phone.balance')}
          value={fmt(planMonth.balance)}
          strong
          negative={cmp(planMonth.balance, '0') < 0}
          {...shared}
        />
      ) : null}
    </div>
  )
}
