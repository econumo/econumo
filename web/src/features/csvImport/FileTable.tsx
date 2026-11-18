import { useCallback, useEffect, useId, useRef, useState } from 'react'
import type { KeyboardEvent } from 'react'
import { Check, ChevronsLeftRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { CollapsedColumn } from './columnLabels'
import type { ParsedFile } from './parse'
import type { StepId } from './prepare'

const SHOWN_ROWS = 6
const OPEN_WIDTH = 224
const FOLDED_WIDTH = 36

// The columns the last table of a file showed folded: the next step's table starts from them and
// folds or unfolds the difference, so moving between steps reads as one table folding like an accordion.
const shownFolded = new WeakMap<ParsedFile, ReadonlySet<number>>()

const prefersReducedMotion = () =>
  typeof window !== 'undefined' && typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches

interface FileTableProps {
  file: ParsedFile
  /** the step's question, the accessible name of the choice */
  label: string
  selected: number | null
  onSelect: (column: number | null) => void
  /** what else this step uses a column for, shown under its header */
  labels?: Record<number, string>
  /** columns other steps answered: shown as a narrow strip that opens that step */
  collapsed?: Record<number, CollapsedColumn>
  onOpenStep?: (step: StepId) => void
  /** file row indexes (0 = first data row) that will be skipped */
  mutedRows?: ReadonlySet<number>
  /** offers "no column" as an answer, under the table */
  noneLabel?: string
}

/** The file as a spreadsheet shows it, header and first rows; a column is chosen by tapping anywhere in it */
export function FileTable({ file, label, selected, onSelect, labels = {}, collapsed = {}, onOpenStep, mutedRows, noneLabel }: FileTableProps) {
  const { t } = useTranslation()
  const baseId = useId()
  const scroller = useRef<HTMLDivElement>(null)
  const headers = useRef<(HTMLButtonElement | null)[]>([])
  const [hovered, setHovered] = useState<number | null>(null)
  const [moreRight, setMoreRight] = useState(false)

  const width = Math.max(file.header.length, ...file.rows.slice(0, SHOWN_ROWS).map((r) => r.length))
  const columns = Array.from({ length: width }, (_, i) => i)
  // the step's own choice stays open even when another step uses the same column
  const isCollapsed = (column: number) => column !== selected && collapsed[column] !== undefined
  const choosable = columns.filter((c) => !isCollapsed(c))
  const rows: { index: number; cells: string[] }[] = []
  file.rows.forEach((cells, index) => {
    if (rows.length < SHOWN_ROWS && cells.some((c) => c.trim() !== '')) rows.push({ index, cells })
  })
  const tabStop = selected !== null && choosable.includes(selected) ? selected : (choosable[0] ?? null)
  const nameOf = (column: number) => file.header[column]?.trim() || t('transactions.import_wizard.column_n', { n: column + 1 })

  const collapsedKey = choosable.join(',')
  const target = columns.filter(isCollapsed)
  const [folded, setFolded] = useState<ReadonlySet<number>>(() =>
    prefersReducedMotion() ? new Set(target) : (shownFolded.get(file) ?? new Set(target)),
  )
  const isFolded = (column: number) => folded.has(column)

  // the change lands two frames after the previous state was painted, so the width transition runs
  useEffect(() => {
    const next = new Set(target)
    shownFolded.set(file, next)
    if (prefersReducedMotion()) {
      setFolded(next)
      return
    }
    let frame = requestAnimationFrame(() => {
      frame = requestAnimationFrame(() => setFolded(next))
    })
    return () => cancelAnimationFrame(frame)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [file, collapsedKey])
  const measure = useCallback(() => {
    const box = scroller.current
    if (box) setMoreRight(box.scrollLeft + box.clientWidth < box.scrollWidth - 1)
  }, [])

  // a column chosen before the step opened may sit off-screen on a phone
  useEffect(() => {
    const box = scroller.current
    const header = selected === null ? null : headers.current[selected]
    const cell = header?.closest('th')
    if (box && cell && (cell.offsetLeft + cell.offsetWidth > box.scrollLeft + box.clientWidth || cell.offsetLeft < box.scrollLeft)) {
      box.scrollLeft = Math.max(cell.offsetLeft - 16, 0)
    }
    measure()
    // only when the step opens: the user's own taps are already in view
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    measure()
    const box = scroller.current
    if (!box || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(measure)
    observer.observe(box)
    return () => observer.disconnect()
  }, [measure, selected, collapsedKey])

  const onKeyDown = (e: KeyboardEvent, column: number) => {
    const at = choosable.indexOf(column)
    let next: number | undefined
    if (e.key === 'ArrowRight' || e.key === 'ArrowDown') next = choosable[(at + 1) % choosable.length]
    else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') next = choosable[(at - 1 + choosable.length) % choosable.length]
    else if (e.key === 'Home') next = choosable[0]
    else if (e.key === 'End') next = choosable[choosable.length - 1]
    if (next === undefined) return
    e.preventDefault()
    headers.current[next]?.focus()
    onSelect(next)
  }

  const tint = (column: number) =>
    column === selected ? 'bg-primary/10' : isFolded(column) ? (column === hovered ? 'bg-econumo-hover' : 'bg-econumo-hover/60') : column === hovered ? 'bg-econumo-hover' : ''

  // every cell's content sits in a box whose width animates, so header and values fold together
  const fold = (column: number) => ({
    style: { maxWidth: isFolded(column) ? FOLDED_WIDTH : OPEN_WIDTH },
    className: cn(
      'overflow-hidden whitespace-nowrap transition-[max-width,padding,opacity] duration-300 ease-out motion-reduce:transition-none',
      isFolded(column) ? 'px-1.5 text-clip' : 'px-3 text-ellipsis',
    ),
  })

  const openStep = (column: number) => {
    const owner = collapsed[column]
    if (owner) onOpenStep?.(owner.step)
  }

  return (
    <div role="radiogroup" aria-label={label} className="flex min-w-0 flex-col gap-2">
      <div className="relative max-w-full">
        <div
          ref={scroller}
          className="overflow-x-auto rounded-lg bg-econumo-card"
          onScroll={measure}
          onTransitionEnd={measure}
          onMouseLeave={() => setHovered(null)}
        >
          <table className="w-max min-w-full border-separate border-spacing-0 text-left text-[13px]">
            <thead>
              <tr>
                {columns.map((column) => {
                  const box = fold(column)
                  const owner = collapsed[column]
                  if (isFolded(column) && owner) {
                    const name = t('transactions.import_wizard.change_column', { role: owner.label })
                    return (
                      <th
                        key={column}
                        scope="col"
                        className={cn('cursor-pointer border-b border-border/60 p-0 align-top font-normal', tint(column))}
                        onClick={() => openStep(column)}
                        onMouseEnter={() => setHovered(column)}
                      >
                        <div {...box} className={cn(box.className, 'py-2')}>
                          <button
                            type="button"
                            aria-label={name}
                            title={`${nameOf(column)} · ${name}`}
                            className="flex cursor-pointer flex-col items-center gap-1 rounded-sm text-xs text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50"
                          >
                            <ChevronsLeftRight aria-hidden className="size-3.5 shrink-0" />
                            <span className="flex items-center gap-1 [writing-mode:vertical-rl]">
                              <Check aria-hidden className="size-3.5 shrink-0" />
                              <span className="max-h-40 truncate">{owner.label}</span>
                            </span>
                          </button>
                        </div>
                      </th>
                    )
                  }
                  const labelId = `${baseId}-label-${column}`
                  return (
                    <th
                      key={column}
                      scope="col"
                      className={cn('cursor-pointer border-b border-border/60 p-0 align-top font-normal', tint(column))}
                      onClick={() => (isCollapsed(column) ? openStep(column) : onSelect(column))}
                      onMouseEnter={() => setHovered(column)}
                    >
                      <div {...box} className={cn(box.className, 'py-2')}>
                        <button
                          ref={(el) => {
                            headers.current[column] = el
                          }}
                          type="button"
                          role="radio"
                          aria-checked={column === selected}
                          aria-describedby={labels[column] ? labelId : undefined}
                          tabIndex={column === tabStop ? 0 : -1}
                          onKeyDown={(e) => onKeyDown(e, column)}
                          className={cn(
                            'flex max-w-full cursor-pointer items-center gap-1 rounded-sm text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/50',
                            column === selected && 'text-primary',
                          )}
                        >
                          <span className="truncate">{nameOf(column)}</span>
                          {column === selected ? <Check aria-hidden className="size-4 shrink-0" /> : null}
                        </button>
                        {labels[column] ? (
                          <span id={labelId} className="block truncate text-xs text-muted-foreground">
                            {labels[column]}
                          </span>
                        ) : null}
                      </div>
                    </th>
                  )
                })}
              </tr>
            </thead>
            <tbody>
              {rows.map(({ index, cells }) => (
                <tr key={index} className={mutedRows?.has(index) ? 'text-muted-foreground/70' : undefined}>
                  {columns.map((column) => {
                    const box = fold(column)
                    const folded = isFolded(column)
                    return (
                      <td
                        key={column}
                        aria-hidden={folded || undefined}
                        title={folded ? undefined : (cells[column] ?? '')}
                        className={cn('cursor-pointer p-0 tabular-nums', tint(column))}
                        onClick={() => (isCollapsed(column) ? openStep(column) : onSelect(column))}
                        onMouseEnter={() => setHovered(column)}
                      >
                        <div {...box} className={cn(box.className, 'py-1.5', folded && 'text-muted-foreground/50')}>
                          {cells[column] ?? ''}
                        </div>
                      </td>
                    )
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {moreRight ? (
          <div aria-hidden className="pointer-events-none absolute inset-y-0 right-0 w-10 rounded-r-lg bg-linear-to-l from-econumo-card to-transparent" />
        ) : null}
      </div>
      {noneLabel ? (
        <button
          type="button"
          role="radio"
          aria-checked={selected === null}
          onClick={() => onSelect(null)}
          className={cn(
            'flex items-center gap-1.5 self-start rounded-full border px-3 py-1.5 text-sm transition-colors outline-none hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50',
            selected === null && 'border-primary bg-primary/5 text-primary',
          )}
        >
          {selected === null ? <Check aria-hidden className="size-4 shrink-0" /> : null}
          {noneLabel}
        </button>
      ) : null}
    </div>
  )
}
