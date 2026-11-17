import { useTranslation } from 'react-i18next'
import { ColumnCards } from '../ColumnCards'
import type { StepBodyProps } from '../wizard'
import { StepTitle } from './StepTitle'

export function DescriptionStep({ state, setAnswers }: StepBodyProps) {
  const { t } = useTranslation()
  const title = t('transactions.import_wizard.description.title')
  return (
    <div className="flex flex-col gap-3">
      <StepTitle>{title}</StepTitle>
      <ColumnCards
        label={title}
        profiles={state.profiles}
        value={state.answers.description}
        onChange={(description) => setAnswers('description', { description })}
        allowNone
      />
    </div>
  )
}
