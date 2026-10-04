import { createContext, useContext } from 'react'
import type { MouseEvent } from 'react'

// One set of columns for every line of the desktop/tablet Budget view (section
// headings, folder lines, rows, children, totals), so headings, sums and amounts
// share their right edges. The first figure column grows to the left for a
// carry-over lead-in; the others are fixed.
// right padding = a small button's own (px-2.5), so the last column ends under the header's Configure
export const LINE = 'group/line flex items-center gap-2 pr-2.5 pl-2 sm:gap-3'
export const NAME_COL = 'flex min-w-0 flex-1 items-center gap-2'
export const FIRST_COL = 'hidden min-w-24 shrink-0 items-baseline justify-end gap-1.5 text-right tabular-nums sm:flex'
export const SECOND_COL = 'flex w-20 shrink-0 justify-end text-right tabular-nums sm:w-24'
export const THIRD_COL = 'flex w-20 shrink-0 justify-end text-right tabular-nums sm:w-28'
/** folder lines sit one step in from their section heading, rows one more */
export const FOLDER_INDENT = 'pl-6'
export const ROW_INDENT = 'pl-6'
export const CHILD_INDENT = 'pl-14 sm:pl-16'
// em dash: a column that carries no value at all, as opposed to a zero
export const EMPTY_CELL = '—'

/** The whole line folds, hover-highlighted like a row. Its own controls (the drag
 *  grip, the folder's plus and menu, an info note) keep their click, and a click in
 *  a portalled menu, which React bubbles through here, never folds. The fold button
 *  itself carries data-fold and no handler: its click, mouse or keyboard, lands here. */
export function foldOnLineClick(onToggle: () => void) {
  return (e: MouseEvent<HTMLElement>) => {
    const target = e.target as HTMLElement
    if (!e.currentTarget.contains(target)) {
      return
    }
    const control = target.closest('button, a, input, [role="menuitem"]')
    if (control && !control.hasAttribute('data-fold')) {
      return
    }
    onToggle()
  }
}

/** the hover and pointer every foldable line shares */
export const FOLD_LINE = 'cursor-pointer rounded-md hover:bg-accent/50'

/** one entry of a line's ⋮ menu */
export interface MenuAction {
  label: string
  onSelect: () => void
  destructive?: boolean
  /** shown, but greyed out: the action exists, the user may not use it here */
  disabled?: boolean
  /** why a disabled action is greyed out, shown after its label: "Edit (no access)" */
  reason?: string
}

/** How a line's controls (its ⋮ menu and drag grip) show: under the pointer with a
 *  mouse, or on every line while a touch screen's edit mode is on. */
export type LineControls = 'hover' | 'always'
export const LineControlsContext = createContext<LineControls>('hover')
export const useLineControls = () => useContext(LineControlsContext)

/** the visibility classes for a line control, given how controls show */
export function lineControlClass(controls: LineControls, hoverGroup: 'line' | 'drag'): string {
  if (controls === 'always') {
    return ''
  }
  const hover = hoverGroup === 'line' ? 'group-hover/line:opacity-100' : 'group-hover/drag:opacity-100'
  return `opacity-0 ${hover} focus-visible:opacity-100 data-[state=open]:opacity-100`
}
