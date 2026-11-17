import { AlertTriangle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useIsPhone } from '@/hooks/useIsPhone'
import { cn } from '@/lib/utils'
import type { StepId } from './prepare'

export interface StepBarItem {
  id: StepId
  label: string
  warning: boolean
  hidden?: boolean
}

interface StepBarProps {
  steps: StepBarItem[]
  current: StepId
  onSelect: (id: StepId) => void
  /** nothing to move between yet: no file is loaded */
  disabled?: boolean
}

export function StepBar({ steps, current, onSelect, disabled = false }: StepBarProps) {
  const { t } = useTranslation()
  const isPhone = useIsPhone()
  const shown = steps.filter((step) => !step.hidden)
  const currentLabel = shown.find((step) => step.id === current)?.label
  const attention = <span className="sr-only">, {t('transactions.import_wizard.needs_attention')}</span>
  const mark = <AlertTriangle aria-hidden className="size-3.5 shrink-0 text-amber-700 dark:text-amber-300" />

  return (
    <nav aria-label={t('transactions.import_wizard.steps_label')} className="flex flex-col gap-1.5">
      <ol className={cn('flex items-center', isPhone ? 'justify-center gap-2' : 'flex-wrap gap-1')}>
        {shown.map((step, index) => {
          const active = step.id === current
          return (
            <li key={step.id}>
              <button
                type="button"
                disabled={disabled}
                aria-current={active ? 'step' : undefined}
                onClick={() => onSelect(step.id)}
                className={cn(
                  'flex items-center gap-1 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50',
                  isPhone
                    ? cn('relative size-8 justify-center rounded-full border', active ? 'border-primary bg-primary text-primary-foreground' : 'bg-econumo-card')
                    : cn('rounded-full px-3 py-1', active ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-econumo-hover'),
                )}
              >
                {isPhone ? (
                  <>
                    <span aria-hidden>{index + 1}</span>
                    <span className="sr-only">{step.label}</span>
                    {step.warning ? <span className="absolute -top-1 -right-1">{mark}</span> : null}
                  </>
                ) : (
                  <>
                    <span>{step.label}</span>
                    {step.warning ? mark : null}
                  </>
                )}
                {step.warning ? attention : null}
              </button>
            </li>
          )
        })}
      </ol>
      {isPhone && currentLabel ? <p aria-hidden className="text-center text-sm">{currentLabel}</p> : null}
    </nav>
  )
}
