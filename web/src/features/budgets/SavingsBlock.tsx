import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { DndContext, PointerSensor, closestCenter, useSensor, useSensors } from '@dnd-kit/core'
import type { DragEndEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ChevronDown, ChevronRight, GripVertical } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { EntityIcon } from '@/components/EntityIcon'
import { afterIdFromDrop } from '@/lib/ordering'
import { moneyFormat } from '@/lib/money'
import type { BudgetCommentDto, BudgetDto, BudgetSavingsElementDto } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { AvailablePill } from './BudgetTable'
import { CommentMarker } from './CommentThread'
import { useBudgetPeriodStore } from './budgetStore'
import { commentCellKey } from './queries'

const FOLD_KEY = 'monthly-savings'

interface SavingsBlockProps {
  budget: BudgetDto
  currencies: CurrencyDto[]
  /** "YYYY-MM-01" */
  selectedDate: string
  /** the rule the budget table applies to limit editing for this month */
  canEdit: boolean
  /** drag handles show only here */
  editMode: boolean
  commentsByCell: Map<string, BudgetCommentDto[]>
  onEditPlanned: (row: BudgetSavingsElementDto) => void
  onOpenComments: (row: BudgetSavingsElementDto) => void
  onMove: (id: Id, afterId: Id | null) => void
}

function SortableSavingsRow({ id, children }: { id: string; children: ReactNode }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id })
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`flex items-center gap-1 ${isDragging ? 'opacity-60' : ''}`}
    >
      <button type="button" aria-label={`move ${id}`} className="cursor-grab touch-none text-muted-foreground" {...attributes} {...listeners}>
        <GripVertical className="size-4" />
      </button>
      <div className="min-w-0 flex-1">{children}</div>
    </div>
  )
}

function SavingsRow({
  row,
  currency,
  editable,
  comments,
  editMode,
  onEditPlanned,
  onOpenComments,
}: {
  row: BudgetSavingsElementDto
  currency: CurrencyDto | undefined
  editable: boolean
  comments: BudgetCommentDto[]
  editMode: boolean
  onEditPlanned: (row: BudgetSavingsElementDto) => void
  onOpenComments: (row: BudgetSavingsElementDto) => void
}) {
  const opts = { showCurrency: false, useNativePrecision: false, maxPrecision: currency?.fractionDigits ?? 2 }
  const planned = moneyFormat(row.budgeted, currency, opts)
  const deleted = row.isArchived === 1
  return (
    <div
      className="flex items-center gap-1.5 rounded-md px-1.5 py-2.5 hover:bg-accent/50 sm:gap-2 sm:px-2"
      data-testid={`savings-row-${row.id}`}
    >
      <span className="flex min-w-0 flex-1 items-center gap-2">
        <span className="hidden w-3.5 shrink-0 sm:block" />
        <EntityIcon name={row.icon} className="text-lg text-muted-foreground" />
        <span className={`truncate text-[15px] ${deleted ? 'text-muted-foreground' : ''}`} title={row.name}>
          {row.name}
        </span>
      </span>
      <span className="relative w-20 text-right text-[15px] tabular-nums sm:w-24" data-testid="savings-planned">
        {editMode ? (
          planned
        ) : (
          // a cell that cannot be edited (guest, pre-start month, deleted account)
          // still opens its thread, like a non-editable budgeted cell in the table
          <button
            type="button"
            className="w-full text-right underline-offset-2 hover:underline"
            aria-label={`${editable ? 'planned' : 'comments'} ${row.name}`}
            onClick={() => (editable ? onEditPlanned(row) : onOpenComments(row))}
          >
            {planned}
          </button>
        )}
        {comments.length > 0 ? <CommentMarker count={comments.length} onOpen={() => onOpenComments(row)} /> : null}
      </span>
      <span className="w-20 text-center text-[15px] tabular-nums text-muted-foreground sm:w-24" data-testid="savings-saved">
        {moneyFormat(row.spent, currency, opts)}
      </span>
      <span className="flex w-20 justify-center sm:w-24">
        <AvailablePill available={row.available} currency={currency} testId="savings-remaining" />
      </span>
      <span className="hidden w-6 text-center text-xs text-muted-foreground sm:block">{currency?.symbol}</span>
    </div>
  )
}

export function SavingsBlock({
  budget,
  currencies,
  selectedDate,
  canEdit,
  editMode,
  commentsByCell,
  onEditPlanned,
  onOpenComments,
  onMove,
}: SavingsBlockProps) {
  const { t } = useTranslation()
  const folded = useBudgetPeriodStore((s) => !!s.planFolds[FOLD_KEY])
  const togglePlanFold = useBudgetPeriodStore((s) => s.togglePlanFold)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))

  const rows = budget.structure.savings
  const { live, deleted } = useMemo(() => {
    const byPosition = (a: BudgetSavingsElementDto, b: BudgetSavingsElementDto) => a.position - b.position
    const all = rows ?? []
    return {
      live: all.filter((r) => r.isArchived === 0).sort(byPosition),
      deleted: all.filter((r) => r.isArchived === 1).sort(byPosition),
    }
  }, [rows])

  // the dropped order holds until the refetched budget replaces it
  const [droppedOrder, setDroppedOrder] = useState<Id[] | null>(null)
  useEffect(() => {
    setDroppedOrder(null)
  }, [budget])

  if (live.length + deleted.length === 0) {
    return null
  }

  const ordered = droppedOrder
    ? droppedOrder.map((id) => live.find((r) => r.id === id)).filter((r): r is BudgetSavingsElementDto => !!r)
    : live
  const liveIds = ordered.map((r) => r.id)

  const handleDragEnd = ({ active, over }: DragEndEvent) => {
    const activeId = String(active.id)
    const overId = over ? String(over.id) : null
    // the block is one folder-less list: only another live savings row is a target
    if (!overId || overId === activeId || !liveIds.includes(overId) || !liveIds.includes(activeId)) {
      return
    }
    const reordered = arrayMove(liveIds, liveIds.indexOf(activeId), liveIds.indexOf(overId))
    setDroppedOrder(reordered)
    onMove(activeId, afterIdFromDrop(reordered, activeId))
  }

  const renderRow = (row: BudgetSavingsElementDto) => (
    <SavingsRow
      row={row}
      currency={currencies.find((c) => c.id === row.currencyId)}
      editable={canEdit && row.isArchived === 0}
      comments={commentsByCell.get(commentCellKey(row.id, selectedDate)) ?? []}
      editMode={editMode}
      onEditPlanned={onEditPlanned}
      onOpenComments={onOpenComments}
    />
  )

  const open = !folded
  return (
    <Collapsible open={open} onOpenChange={() => togglePlanFold(FOLD_KEY)}>
      <section className="mb-[max(env(safe-area-inset-bottom),0.75rem)] mt-3 rounded-md border p-1.5 sm:p-2" data-testid="budget-savings-block">
        <div className="flex items-center gap-1.5 px-1.5 pb-1 sm:gap-2 sm:px-2">
          <CollapsibleTrigger asChild>
            <button
              type="button"
              className="flex min-w-0 flex-1 items-center gap-1.5 text-left sm:gap-2"
              aria-expanded={open}
              title={t(open ? 'common.button.collapse.label' : 'common.button.expand.label')}
            >
              {open ? <ChevronDown className="size-3.5 shrink-0 text-muted-foreground" /> : <ChevronRight className="size-3.5 shrink-0 text-muted-foreground" />}
              <span className="min-w-0 truncate text-sm font-medium">{t('budgets.page.savings.title')}</span>
            </button>
          </CollapsibleTrigger>
          {/* long translations (pl, ru) outgrow the phone column: truncate, full text on hover */}
          {(['planned', 'saved', 'remaining'] as const).map((col) => (
            <span
              key={col}
              title={t(`budgets.page.savings.${col}`)}
              className={`w-20 truncate text-[11px] uppercase tracking-wide text-muted-foreground sm:w-24 ${col === 'planned' ? 'text-right' : 'text-center'}`}
            >
              {t(`budgets.page.savings.${col}`)}
            </span>
          ))}
          <span className="hidden w-6 sm:block" />
        </div>
        <CollapsibleContent>
          {editMode ? (
            <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
              <SortableContext items={liveIds} strategy={verticalListSortingStrategy}>
                {ordered.map((row) => (
                  <SortableSavingsRow key={row.id} id={row.id}>
                    {renderRow(row)}
                  </SortableSavingsRow>
                ))}
              </SortableContext>
            </DndContext>
          ) : (
            ordered.map((row) => <div key={row.id}>{renderRow(row)}</div>)
          )}
          {deleted.map((row) => (
            // no grip, but the grip's width is kept so the columns stay aligned
            <div key={row.id} className={editMode ? 'flex items-center gap-1' : undefined}>
              {editMode ? <span className="w-4 shrink-0" /> : null}
              <div className="min-w-0 flex-1">{renderRow(row)}</div>
            </div>
          ))}
        </CollapsibleContent>
      </section>
    </Collapsible>
  )
}
