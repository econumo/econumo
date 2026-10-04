// One set of columns for every line of the desktop/tablet Budget view (section
// headings, folder lines, rows, children, totals), so headings, sums and amounts
// share their right edges. The first figure column grows to the left for a
// carry-over lead-in; the others are fixed.
export const LINE = 'flex items-center gap-2 px-2 sm:gap-3'
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
