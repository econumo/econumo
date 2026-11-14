// debt dates are bare Y-m-d (a payment's carries a time); read as local days
function localDay(date: string): Date {
  const [y, m, d] = date.slice(0, 10).split('-').map(Number)
  return new Date(y, m - 1, d || 1)
}

/** "Jul 2050" */
export function debtMonth(date: string, lang: string): string {
  return new Intl.DateTimeFormat(lang, { month: 'short', year: 'numeric' }).format(localDay(date))
}

/** "Jul 5, 2050" */
export function debtDay(date: string, lang: string): string {
  return new Intl.DateTimeFormat(lang, { day: 'numeric', month: 'short', year: 'numeric' }).format(localDay(date))
}
