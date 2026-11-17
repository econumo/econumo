import { useTranslation } from 'react-i18next'
import type { StepBodyProps } from '../wizard'
import { PlainColumnPicker } from './PlainColumnPicker'
import { StepTitle } from './StepTitle'

export function AmountStep({ state, setAnswers }: StepBodyProps) {
  const { t } = useTranslation()
  const title = t('transactions.import_wizard.amount.title')
  const { amount } = state.answers
  return (
    <div className="flex flex-col gap-2">
      <StepTitle>{title}</StepTitle>
      <PlainColumnPicker
        label={title}
        profiles={state.profiles}
        value={amount.mode === 'single' ? amount.column : null}
        onChange={(column) => setAnswers('amount', { amount: { ...amount, mode: 'single', column } })}
      />
    </div>
  )
}
