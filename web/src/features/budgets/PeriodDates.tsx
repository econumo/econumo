import { useTranslation } from 'react-i18next'
import { periodDatesLabel } from './period'

/** the dates a shifted budget month covers; a calendar month needs no range */
export function PeriodDates({ periodKey, startDay }: { periodKey: string; startDay: number }) {
  const { i18n } = useTranslation()
  const label = periodDatesLabel(periodKey, startDay, i18n.language)
  if (label === null) {
    return null
  }
  return (
    <span data-testid="period-dates" className="shrink-0 whitespace-nowrap px-1 text-xs text-muted-foreground tabular-nums">
      {label}
    </span>
  )
}
