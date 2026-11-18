import { useCallback, useEffect, useId, useRef, useState } from 'react'
import type { KeyboardEvent } from 'react'
import { Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { CollapsedColumn } from './columnLabels'
import type { ParsedFile } from './parse'
import type { StepId } from './prepare'

const SHOWN_ROWS = 6

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

  const tint = (column: number) => (column === selected ? 'bg-primary/10' : column === hovered ? 'bg-econumo-hover' : '')

  return (
    <div role="radiogroup" aria-label={label} className="flex min-w-0 flex-col gap-2">
      <div className="relative max-w-full">
        <div ref={scroller} className="overflow-x-auto rounded-lg bg-econumo-card" onScroll={measure} onMouseLeave={() => setHovered(null)}>
          <table className="w-max min-w-full border-separate border-spacing-0 text-left text-[13px]">
            <thead>
              <tr>
                {columns.map((column) => {
                  const strip = isCollapsed(column) ? collapsed[column] : null
                  if (strip) {
                    const name = t('transactions.import_wizard.change_column', { role: strip.label })
                    return (
                      <th key={column} scope="col" className="w-9 border-b border-border/60 bg-econumo-hover/70 p-0 align-top font-normal">
                        <button
                          type="button"
                          aria-label={name}
                          title={`${nameOf(column)} · ${name}`}
                          onClick={() => onOpenStep?.(strip.step)}
                          className="flex w-9 cursor-pointer items-center justify-start gap-1 py-3 text-xs text-muted-foreground outline-none hover:bg-econumo-hover hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 [writing-mode:vertical-rl]"
                        >
                          <Check aria-hidden className="size-3.5 shrink-0" />
                          <span className="max-h-48 truncate">{strip.label}</span>
                        </button>
                      </th>
                    )
                  }
                  const labelId = `${baseId}-label-${column}`
                  return (
                    <th
                      key={column}
                      scope="col"
                      className={cn('cursor-pointer border-b border-border/60 px-3 py-2 align-top font-normal', tint(column))}
                      onClick={() => onSelect(column)}
                      onMouseEnter={() => setHovered(column)}
                    >
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
                          'flex max-w-56 cursor-pointer items-center gap-1 rounded-sm text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/50',
                          column === selected && 'text-primary',
                        )}
                      >
                        <span className="truncate">{nameOf(column)}</span>
                        {column === selected ? <Check aria-hidden className="size-4 shrink-0" /> : null}
                      </button>
                      {labels[column] ? (
                        <span id={labelId} className="block text-xs text-muted-foreground">
                          {labels[column]}
                        </span>
                      ) : null}
                    </th>
                  )
                })}
              </tr>
            </thead>
            <tbody>
              {rows.map(({ index, cells }) => (
                <tr key={index} className={mutedRows?.has(index) ? 'text-muted-foreground/70' : undefined}>
                  {columns.map((column) => {
                    const strip = isCollapsed(column) ? collapsed[column] : null
                    return strip ? (
                      <td key={column} aria-hidden className="w-9 cursor-pointer bg-econumo-hover/70 p-0" onClick={() => onOpenStep?.(strip.step)} />
                    ) : (
                      <td
                        key={column}
                        title={cells[column] ?? ''}
                        className={cn('max-w-56 cursor-pointer truncate px-3 py-1.5 whitespace-nowrap tabular-nums', tint(column))}
                        onClick={() => onSelect(column)}
                        onMouseEnter={() => setHovered(column)}
                      >
                        {cells[column] ?? ''}
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
            'flex items-center gap-1 self-start rounded-sm text-sm underline-offset-2 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring/50',
            selected === null ? 'text-primary' : 'text-muted-foreground',
          )}
        >
          {selected === null ? <Check aria-hidden className="size-4 shrink-0" /> : null}
          {noneLabel}
        </button>
      ) : null}
    </div>
  )
}
