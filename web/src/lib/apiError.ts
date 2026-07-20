import { isAxiosError } from 'axios'
import i18n from '@/app/i18n'

interface ErrEnvelope {
  success: false
  message: string
  errors?: Record<string, string[]>
}

// The backend renders message/errors in the caller's language (it resolves
// Accept-Language, falling back to the stored user locale). Prefer the first
// field error — with field errors present the top-level message is the
// generic "Form validation error" label — then the message itself.
export function apiErrorMessage(err: unknown): string {
  const data = isAxiosError(err) ? (err.response?.data as ErrEnvelope | undefined) : undefined
  if (data) {
    const firstField = Object.values(data.errors ?? {})[0]?.[0]
    if (firstField) return firstField
    if (data.message) return data.message
  }
  return i18n.t('common.app.error')
}

// Per-field messages for inline form errors, as rendered by the backend.
export function apiFieldErrors(err: unknown, field: string): string[] | undefined {
  const data = isAxiosError(err) ? (err.response?.data as ErrEnvelope | undefined) : undefined
  return data?.errors?.[field]
}
