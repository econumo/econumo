import { useTranslation } from 'react-i18next'
import { unusedColumns, type StepBodyProps } from '../wizard'
import { StepTitle } from './StepTitle'

export function ExtrasStep({ state }: StepBodyProps) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col gap-2">
      <StepTitle>{t('transactions.import_wizard.extras.title')}</StepTitle>
      <ul className="flex flex-col gap-1 text-sm">
        {unusedColumns(state.answers, state.profiles).map((p) => (
          <li key={p.index}>
            {p.name}: <span className="text-muted-foreground">{p.samples.join(', ')}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}
