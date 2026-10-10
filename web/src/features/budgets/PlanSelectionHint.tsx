import { useLayoutEffect, useRef } from 'react'
import type { RefObject } from 'react'
import { useTranslation } from 'react-i18next'
import type { CurrencyDto } from '@/api/dto/currency'
import { moneyFormat } from '@/lib/money'
import { pluralPick } from '@/lib/plural'
import type { PlanSelectionSum } from './planMath'

const GAP_PX = 4

/** A multi-cell selection's total, floating off the selection's bottom-right corner
 *  (or its top-right when the sticky balance row would cover it). It lives in the
 *  grid's scroll content, so it scrolls with the cells it describes. */
export function PlanSelectionHint({
  sum,
  cellIds,
  currency,
  containerRef,
}: {
  sum: PlanSelectionSum
  /** the DOM ids of the selected cells on screen */
  cellIds: string[]
  currency: CurrencyDto | undefined
  containerRef: RefObject<HTMLDivElement | null>
}) {
  const { t, i18n } = useTranslation()
  const ref = useRef<HTMLDivElement | null>(null)
  const fmt = (v: string) => moneyFormat(v, currency, { showCurrency: false, useNativePrecision: false })

  // Placed by measuring rather than by layout: the selection can be any set of cells
  // across rows and folders, so only their rendered boxes say where it ends.
  useLayoutEffect(() => {
    const hint = ref.current
    const scroller = containerRef.current
    if (!hint || !scroller) {
      return
    }
    let top = Infinity
    let bottom = -Infinity
    let right = -Infinity
    for (const id of cellIds) {
      const box = document.getElementById(id)?.getBoundingClientRect()
      if (box) {
        top = Math.min(top, box.top)
        bottom = Math.max(bottom, box.bottom)
        right = Math.max(right, box.right)
      }
    }
    if (right === -Infinity) {
      return
    }
    const view = scroller.getBoundingClientRect()
    const footer = scroller.querySelector<HTMLElement>('[data-testid="plan-balance-row"]')
    const header = scroller.querySelector<HTMLElement>('[data-testid="plan-month-header"]')
    const floor = footer ? footer.getBoundingClientRect().top : view.bottom
    const ceiling = header ? header.getBoundingClientRect().bottom : view.top
    const h = hint.offsetHeight
    const below = bottom + GAP_PX + h <= floor || top - GAP_PX - h < ceiling
    const y = below ? bottom + GAP_PX : top - GAP_PX - h
    hint.style.top = `${y - view.top + scroller.scrollTop}px`
    hint.style.left = `${right - view.left + scroller.scrollLeft}px`
  })

  return (
    <div
      ref={ref}
      role="status"
      data-testid="plan-selection-hint"
      className="pointer-events-none absolute z-10 -translate-x-full rounded-md border bg-popover px-2.5 py-1 text-xs whitespace-nowrap text-popover-foreground shadow-md tabular-nums"
    >
      <span data-testid="plan-selection-count" className="text-muted-foreground">
        {pluralPick(t('budgets.page.plan.selection.count'), sum.count, i18n.language)}
      </span>
      <span className="text-muted-foreground"> · </span>
      <span data-testid="plan-selection-planned">
        <span className="text-muted-foreground">{t('budgets.page.plan.selection.planned')}</span> {fmt(sum.planned)}
      </span>
      <span className="text-muted-foreground"> · </span>
      <span data-testid="plan-selection-actual">
        <span className="text-muted-foreground">{t('budgets.page.plan.selection.actual')}</span> {fmt(sum.actual)}
      </span>
    </div>
  )
}
