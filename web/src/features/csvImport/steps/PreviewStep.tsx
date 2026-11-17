import { useTranslation } from 'react-i18next'
import type { StepBodyProps } from '../wizard'
import { StepTitle } from './StepTitle'

export function PreviewStep(_props: StepBodyProps) {
  const { t } = useTranslation()
  return <StepTitle>{t('transactions.import_wizard.preview.title')}</StepTitle>
}
