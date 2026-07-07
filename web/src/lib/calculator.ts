export function sanitizeInput(value: string | number): string {
  let s = value.toString()
  s = s.replace(/[^0-9+\-*/=.,]/g, '')
  s = s.replace(/,/g, '.')
  s = s.replace(/(?<!\d)[+*/.]{2,}/g, '')
  const parts = s.split(/([+\-*/])/)
  const sanitizedParts = parts.map((part) => {
    if (part.includes('.')) {
      return part
    }
    return part.replace(/\b0+(\d+)/g, '$1')
  })
  return sanitizedParts.join('')
}

type Token = { kind: 'num'; value: number } | { kind: 'op'; op: '+' | '-' | '*' | '/' }

// Tokenizer + recursive-descent evaluator replacing the Vue app's eval().
// A leading '-' (or one following an operator) binds as a unary minus.
function tokenize(formula: string): Token[] | null {
  const tokens: Token[] = []
  let i = 0
  while (i < formula.length) {
    const ch = formula[i]
    if (ch === '*' || ch === '/') {
      tokens.push({ kind: 'op', op: ch })
      i++
      continue
    }
    if (ch === '+' || ch === '-') {
      const prev = tokens[tokens.length - 1]
      if (!prev || prev.kind === 'op') {
        // unary sign, as eval() accepted ("+5", "5*-2")
        const m = /^[+-]\d+(\.\d+)?/.exec(formula.slice(i))
        if (!m) {
          return null
        }
        tokens.push({ kind: 'num', value: Number(m[0]) })
        i += m[0].length
        continue
      }
      tokens.push({ kind: 'op', op: ch })
      i++
      continue
    }
    const m = /^\d+(\.\d+)?|^\.\d+/.exec(formula.slice(i))
    if (!m) {
      return null
    }
    tokens.push({ kind: 'num', value: Number(m[0]) })
    i += m[0].length
    continue
  }
  return tokens
}

function compute(tokens: Token[]): number | null {
  if (tokens.length === 0 || tokens[0].kind !== 'num' || tokens.length % 2 === 0) {
    return null
  }
  for (let i = 1; i < tokens.length; i += 2) {
    if (tokens[i].kind !== 'op' || tokens[i + 1]?.kind !== 'num') {
      return null
    }
  }
  // first pass: * and /
  const flat: (number | '+' | '-')[] = []
  let acc = (tokens[0] as { kind: 'num'; value: number }).value
  for (let i = 1; i < tokens.length; i += 2) {
    const op = (tokens[i] as { kind: 'op'; op: string }).op
    const rhs = (tokens[i + 1] as { kind: 'num'; value: number }).value
    if (op === '*') {
      acc *= rhs
    } else if (op === '/') {
      acc /= rhs
    } else {
      flat.push(acc, op as '+' | '-')
      acc = rhs
    }
  }
  flat.push(acc)
  let result = flat[0] as number
  for (let i = 1; i < flat.length; i += 2) {
    const op = flat[i] as '+' | '-'
    const rhs = flat[i + 1] as number
    result = op === '+' ? result + rhs : result - rhs
  }
  return result
}

function evaluate(formula: string): number | null {
  const tokens = tokenize(formula)
  if (tokens === null) {
    return null
  }
  const result = compute(tokens)
  if (result === null || Number.isNaN(result)) {
    return null
  }
  return result
}

export function validateFormula(formula: string | number): boolean {
  const s = formula.toString()
  if (s === '') {
    return true
  }
  const body = s.replace('=', '')
  if (body === '') {
    return true
  }
  return evaluate(body) !== null
}

export function evaluateFormula(value: string | number): string {
  const s = value.toString()
  if (!s.includes('=')) {
    return s
  }
  const body = s.replace('=', '')
  const result = evaluate(body)
  if (result === null) {
    return body
  }
  return roundToPrecision(result, 10).toString()
}

function roundToPrecision(num: number, precision: number): number {
  const factor = Math.pow(10, precision)
  return Math.round(num * factor) / factor
}
