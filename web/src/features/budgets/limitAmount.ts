import { evaluateFormula, sanitizeInput, validateFormula } from '@/lib/calculator'
import { normalizeNumber } from '@/lib/money'

// Unified set-limit amount rule (approved divergence — Vue's two paths disagree):
// empty / 0 / NaN clears the limit (null); otherwise the normalized decimal string.
export function limitAmountFromInput(raw: string): { ok: true; amount: string | null } | { ok: false } {
  const trimmed = raw.trim()
  if (trimmed === '') {
    return { ok: true, amount: null }
  }
  const sanitized = sanitizeInput(trimmed)
  if (!validateFormula(sanitized)) {
    return { ok: false }
  }
  const evaluated = Number(evaluateFormula(sanitized + '='))
  if (Number.isNaN(evaluated)) {
    return { ok: false }
  }
  if (evaluated === 0) {
    return { ok: true, amount: null }
  }
  return { ok: true, amount: normalizeNumber(evaluated) }
}
