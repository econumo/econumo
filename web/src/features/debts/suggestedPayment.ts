import { add, cmp, div, isZero, toFixedString } from '@/lib/decimal'
import type { Numeric } from '@/lib/decimal'

export interface SuggestedPaymentInput {
  /** the current period's plan ('' = none) and actual spending */
  planned: Numeric
  actual: Numeric
  /** actual spending of the complete periods before the current one, up to 3 */
  previous: Numeric[]
  fractionDigits: number
}

const PREVIOUS_PERIODS = 3

/** The payment a new debt starts with: the larger of this period's plan and
    spending, else the average spending of the last three complete periods (all
    three count, a quiet one as 0). '' when there is nothing to go on. */
export function suggestedPayment({ planned, actual, previous, fractionDigits }: SuggestedPaymentInput): string {
  const current = cmp(planned, actual) >= 0 ? planned : actual
  if (!isZero(current) && cmp(current, 0) > 0) {
    return toFixedString(current, fractionDigits)
  }
  const sum = previous.slice(-PREVIOUS_PERIODS).reduce<string>((acc, v) => add(acc, v), '0')
  if (cmp(sum, 0) <= 0) {
    return ''
  }
  const average = toFixedString(div(sum, PREVIOUS_PERIODS), fractionDigits)
  return isZero(average) ? '' : average
}
