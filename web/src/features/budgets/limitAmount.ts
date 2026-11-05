import { evaluateFormula, sanitizeInput, validateFormula } from '@/lib/calculator'
import { tryNormalize } from '@/lib/decimal'

// Unified set-limit amount rule: empty clears the plan, unparseable is rejected;
// otherwise the normalized decimal string (0 included: a plan of 0 is a plan). Plain decimals skip the float
// calculator so large limits keep every digit.
export function limitAmountFromInput(raw: string): { ok: true; amount: string | null } | { ok: false } {
  const trimmed = raw.trim()
  if (trimmed === '') {
    return { ok: true, amount: null }
  }
  const sanitized = sanitizeInput(trimmed)
  if (!validateFormula(sanitized)) {
    return { ok: false }
  }
  const evaluated = /^-?\d+(\.\d+)?$/.test(sanitized) ? tryNormalize(sanitized) : tryNormalize(evaluateFormula(sanitized + '='))
  if (evaluated === null) {
    return { ok: false }
  }
  return { ok: true, amount: evaluated }
}
