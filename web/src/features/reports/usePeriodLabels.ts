import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { periodLabels } from './reportPeriod'
import type { PeriodLabels } from './reportPeriod'

export function usePeriodLabels(): PeriodLabels {
  const { t, i18n } = useTranslation()
  return useMemo(
    () => periodLabels(i18n.language, (quarter, year) => t('reports.period.quarter', { quarter, year })),
    [i18n.language, t],
  )
}
