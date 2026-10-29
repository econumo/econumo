import { useTranslation } from 'react-i18next'
import { periodWindow } from './period'

const localDate = (iso: string): Date => {
  const [y, m, d] = iso.split('-').map(Number)
  return new Date(y, m - 1, d)
}

/** the dates a shifted budget month covers; a calendar month needs no range */
export function PeriodDates({ periodKey, startDay }: { periodKey: string; startDay: number }) {
  const { i18n } = useTranslation()
  if (startDay === 1) {
    return null
  }
  const { start, end } = periodWindow(periodKey, startDay)
  const last = localDate(end)
  last.setDate(last.getDate() - 1)
  const fmt = new Intl.DateTimeFormat(i18n.language, { day: 'numeric', month: 'short' })
  return (
    <span data-testid="period-dates" className="shrink-0 whitespace-nowrap px-1 text-xs text-muted-foreground tabular-nums">
      {fmt.format(localDate(start))} – {fmt.format(last)}
    </span>
  )
}
