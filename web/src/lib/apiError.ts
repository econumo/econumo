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

// The login 403 is the email-verification signal (no other 403 exists on that
// route); the dialog flow keys off the status alone.
export function isForbidden(err: unknown): boolean {
  return isAxiosError(err) && err.response?.status === 403
}

// Seconds from the standard Retry-After header, or 0 when absent/unparseable.
// The email-verification 403 carries it so the resend countdown is already
// correct when the dialog opens.
export function retryAfterSeconds(err: unknown): number {
  if (!isAxiosError(err)) return 0
  const raw = err.response?.headers?.['retry-after']
  const seconds = Number(raw)
  return Number.isFinite(seconds) && seconds > 0 ? seconds : 0
}
