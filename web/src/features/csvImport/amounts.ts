export type DecimalSeparator = '.' | ','

const CURRENCY_EDGES_START = /^[\p{L}\p{Sc}\s]+/u
const CURRENCY_EDGES_END = /[\p{L}\p{Sc}\s]+$/u
const MINUS = '-−'

function stripEdges(s: string): string {
  return s.replace(CURRENCY_EDGES_START, '').replace(CURRENCY_EDGES_END, '')
}

export function parseAmountCell(value: string, sep: DecimalSeparator): string | null {
  let s = stripEdges(value)
  if (s === '') return null

  let negative = false
  if (s.startsWith('(') && s.endsWith(')')) {
    negative = true
    s = s.slice(1, -1)
  }
  s = stripEdges(s)
  if (MINUS.includes(s[0] ?? '') || s[0] === '+') {
    negative = negative || s[0] !== '+'
    s = s.slice(1)
  } else if (s.endsWith('-') || s.endsWith('−')) {
    negative = true
    s = s.slice(0, -1)
  }
  s = stripEdges(s)

  if (!/^[\d.,\s'’]+$/.test(s) || !/\d/.test(s)) return null
  s = s.replace(/[\s'’]/g, '')

  s = sep === ',' ? s.replace(/\./g, '').replace(',', '.') : s.replace(/,/g, '')
  if (!/^(\d+\.?\d*|\.\d+)$/.test(s)) return null

  if (s.startsWith('.')) s = '0' + s
  if (s.endsWith('.')) s = s.slice(0, -1)
  if (negative && /[1-9]/.test(s)) s = '-' + s
  return s
}

export function detectDecimalSeparator(values: string[]): {
  sep: DecimalSeparator
  ambiguous: boolean
  example: string | null
} {
  let ambiguousExample: string | null = null

  for (const raw of values) {
    const s = raw.replace(/[^\d.,]/g, '')
    const lastDot = s.lastIndexOf('.')
    const lastComma = s.lastIndexOf(',')
    if (lastDot === -1 && lastComma === -1) continue

    if (lastDot !== -1 && lastComma !== -1) {
      return { sep: lastComma > lastDot ? ',' : '.', ambiguous: false, example: null }
    }

    const mark: DecimalSeparator = lastDot !== -1 ? '.' : ','
    const other: DecimalSeparator = mark === '.' ? ',' : '.'
    const count = s.split(mark).length - 1
    if (count > 1) return { sep: other, ambiguous: false, example: null }
    if (s.length - s.lastIndexOf(mark) - 1 === 3) {
      ambiguousExample ??= raw.trim()
      continue
    }
    return { sep: mark, ambiguous: false, example: null }
  }

  return { sep: '.', ambiguous: ambiguousExample !== null, example: ambiguousExample }
}

function negate(amount: string): string {
  if (amount.startsWith('-')) return amount.slice(1)
  return /[1-9]/.test(amount) ? '-' + amount : amount
}

const isNonZero = (amount: string) => /[1-9]/.test(amount)

export function signedAmount(
  input: { single?: string | null; inflow?: string | null; outflow?: string | null },
  mode: 'single' | 'dual',
  flip: boolean,
  sep: DecimalSeparator,
): string | null {
  if (mode === 'single') {
    const amount = parseAmountCell(input.single ?? '', sep)
    if (amount === null) return null
    return flip ? negate(amount) : amount
  }

  const inflow = parseAmountCell(input.inflow ?? '', sep)
  if (inflow !== null && isNonZero(inflow)) return inflow.replace(/^-/, '')
  const outflow = parseAmountCell(input.outflow ?? '', sep)
  if (outflow !== null) return isNonZero(outflow) ? '-' + outflow.replace(/^-/, '') : outflow
  return null
}
