import { useTranslation } from 'react-i18next'
import type { StepBodyProps } from '../wizard'
import { PlainColumnPicker } from './PlainColumnPicker'
import { StepTitle } from './StepTitle'

export function DescriptionStep({ state, setAnswers }: StepBodyProps) {
  const { t } = useTranslation()
  const title = t('transactions.import_wizard.description.title')
  return (
    <div className="flex flex-col gap-2">
      <StepTitle>{title}</StepTitle>
      <PlainColumnPicker
        label={title}
        profiles={state.profiles}
        value={state.answers.description}
        onChange={(description) => setAnswers('description', { description })}
      />
    </div>
  )
}
