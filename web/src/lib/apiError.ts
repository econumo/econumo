import { isAxiosError } from 'axios'
import i18n from '@/app/i18n'

interface CodeRef {
  code: string
  params?: Record<string, unknown>
}

interface ErrEnvelope {
  success: false
  message: string
  errors?: Record<string, string[]>
  errorCodes?: Record<string, CodeRef[]>
  messageCode?: string
  messageParams?: Record<string, unknown>
}

function translate(ref: CodeRef): string | null {
  const key = `errors.${ref.code}`
  return i18n.exists(key) ? i18n.t(key, ref.params) : null
}

// Prefers the envelope's machine codes (translated via the catalogue) and
// falls back to the frozen English message so unknown codes stay visible.
export function apiErrorMessage(err: unknown): string {
  const data = isAxiosError(err) ? (err.response?.data as ErrEnvelope | undefined) : undefined
  if (data) {
    if (data.messageCode) {
      const msg = translate({ code: data.messageCode, params: data.messageParams })
      if (msg) return msg
    }
    const firstField = Object.values(data.errorCodes ?? {})[0]?.[0]
    if (firstField) {
      const msg = translate(firstField)
      if (msg) return msg
    }
    if (data.message) return data.message
  }
  return i18n.t('common.app.error')
}
