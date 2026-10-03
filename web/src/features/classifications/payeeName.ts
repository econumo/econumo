import type { TFunction } from 'i18next'
import { isNotEmpty, isValidPayeeName } from '@/lib/validation'

export function validatePayeeName(t: TFunction) {
  return (value: string): string | null => {
    if (!isNotEmpty(value)) {
      return t('classifications.payees.forms.payee.name.validation.required_field')
    }
    if (!isValidPayeeName(value)) {
      return t('classifications.payees.forms.payee.name.validation.invalid_name')
    }
    return null
  }
}
