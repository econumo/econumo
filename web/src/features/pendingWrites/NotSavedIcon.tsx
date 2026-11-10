import { CloudOff } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { Id } from '@/api/types'
import { usePendingLimit } from './hooks'

export function NotSavedIcon({ className = '' }: { className?: string }) {
  const { t } = useTranslation()
  return (
    <span role="img" aria-label={t('budgets.limit.not_saved')} title={t('budgets.limit.not_saved')} className={`inline-flex shrink-0 text-muted-foreground ${className}`}>
      <CloudOff aria-hidden="true" className="size-3" />
    </span>
  )
}

export function PendingLimitMark({ budgetId, elementId, period, className }: { budgetId: Id; elementId: Id; period: string; className?: string }) {
  return usePendingLimit(budgetId, elementId, period) ? <NotSavedIcon className={className} /> : null
}
