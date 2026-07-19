import Big from 'big.js'

// Mirrors the backend's vo.DecimalNumber (internal/shared/vo/decimal.go):
// scale 8; Mul/Div TRUNCATE toward zero; Round is half-away-from-zero.
// All exports take and return decimal strings — Big instances never escape,
// and no other module may import big.js.
const SCALE = 8

// Scoped constructor so div() truncates toward zero at scale 8 without
// mutating the library-global Big config.
const BigD = Big()
BigD.DP = SCALE
BigD.RM = BigD.roundDown

export type Numeric = string | number | null | undefined

// Truncate to scale 8 (toward zero), mirroring backend vo.NewDecimal.
const truncate8 = (b: Big): Big => b.round(SCALE, BigD.roundDown)

// Constructor: truncate every operand at construction (vo.NewDecimal parity).
const big = (v: Numeric): Big => {
  if (v === null || v === undefined) return new BigD(0)
  const s = String(v).trim()
  if (s === '') return new BigD(0)
  try {
    return truncate8(new BigD(s))
  } catch {
    return new BigD(0)
  }
}

// toFixed() with no argument always renders normal notation (never exponential).
const toPlain = (b: Big): string => {
  const s = b.toFixed()
  if (!s.includes('.')) return s
  return s.replace(/0+$/, '').replace(/\.$/, '')
}

export function tryNormalize(raw: string): string | null {
  const s = raw.trim()
  if (s === '') return null
  try {
    return toPlain(truncate8(new BigD(s)))
  } catch {
    return null
  }
}

export function normalize(v: Numeric): string {
  // big(v) already truncates at scale 8; no need to truncate again.
  // Note: -0 collapses to 0 during truncation (matching backend vo normalization).
  return toPlain(big(v))
}

export const add = (a: Numeric, b: Numeric): string => toPlain(big(a).plus(big(b)))
export const sub = (a: Numeric, b: Numeric): string => toPlain(big(a).minus(big(b)))
export const mul = (a: Numeric, b: Numeric): string => toPlain(truncate8(big(a).times(big(b))))

// Division by zero yields '0' (display-safe) where the backend panics.
export const div = (a: Numeric, b: Numeric): string => {
  const den = big(b)
  if (den.eq(0)) return '0'
  return toPlain(big(a).div(den))
}

export const cmp = (a: Numeric, b: Numeric): -1 | 0 | 1 => big(a).cmp(big(b)) as -1 | 0 | 1
export const isZero = (a: Numeric): boolean => big(a).eq(0)
export const isNegative = (a: Numeric): boolean => big(a).lt(0)
export const abs = (a: Numeric): string => toPlain(big(a).abs())

export const round = (a: Numeric, precision: number): string =>
  toPlain(big(a).round(Math.max(0, precision), BigD.roundHalfUp))

export const toFixedString = (a: Numeric, digits: number): string =>
  big(a).round(digits, BigD.roundHalfUp).toFixed(digits)
