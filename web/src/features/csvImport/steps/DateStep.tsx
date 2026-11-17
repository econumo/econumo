import { useTranslation } from 'react-i18next'
import type { StepBodyProps } from '../wizard'
import { PlainColumnPicker } from './PlainColumnPicker'
import { StepTitle } from './StepTitle'

export function DateStep({ state, setAnswers }: StepBodyProps) {
  const { t } = useTranslation()
  const title = t('transactions.import_wizard.date.title')
  return (
    <div className="flex flex-col gap-2">
      <StepTitle>{title}</StepTitle>
      <PlainColumnPicker
        label={title}
        profiles={state.profiles}
        value={state.answers.date.column}
        onChange={(column) => setAnswers('date', { date: { ...state.answers.date, column } })}
      />
    </div>
  )
}
