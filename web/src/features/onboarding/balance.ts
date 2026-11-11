function separators(locale: string): { decimal: string; group: string } {
  let parts: Intl.NumberFormatPart[]
  try {
    parts = new Intl.NumberFormat(locale).formatToParts(12345.6)
  } catch {
    parts = new Intl.NumberFormat('en').formatToParts(12345.6)
  }
  const decimal = parts.find((p) => p.type === 'decimal')?.value ?? '.'
  const group = parts.find((p) => p.type === 'group')?.value ?? ''
  // a space-like group mark is stripped with the rest of the whitespace
  return { decimal, group: /\s/.test(group) ? '' : group }
}

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

// Reads a balance the way the UI language writes numbers. A mark that is
// neither the locale's decimal nor its group mark is taken as a decimal mark,
// unless exactly three digits follow it: "2.500" typed into a comma-decimal
// locale could mean either, and a wrong guess would be locked in once the
// account is created, so it is refused instead.
export function parseBalance(raw: string, locale: string): string | null {
  const s = raw.replace(/[\s  ]/g, '')
  if (s === '') return '0'
  const { decimal, group } = separators(locale)
  const sign = s.startsWith('-') ? '-' : ''
  const body = sign ? s.slice(1) : s
  const marks = [decimal, group, '.', ','].filter((m, i, all) => m !== '' && all.indexOf(m) === i)
  if (!new RegExp(`^(?:\\d|${marks.map(escape).join('|')})+$`).test(body)) return null

  const grouped = (int: string): string | null => {
    if (/^\d*$/.test(int)) return int
    if (group !== '' && new RegExp(`^\\d{1,3}(?:${escape(group)}\\d{3})+$`).test(int)) return int.split(group).join('')
    return null
  }
  const result = (int: string, frac?: string): string | null => {
    if (int === '' && frac === undefined) return null
    return `${sign}${int || '0'}${frac !== undefined ? `.${frac}` : ''}`
  }

  const decimalAt = body.indexOf(decimal)
  if (decimalAt !== -1) {
    const frac = body.slice(decimalAt + 1)
    const int = grouped(body.slice(0, decimalAt))
    if (int === null || !/^\d+$/.test(frac)) return null
    return result(int, frac)
  }

  const int = grouped(body)
  if (int !== null) return result(int)

  const foreign = marks.filter((m) => m !== decimal && m !== group && body.includes(m))
  if (foreign.length !== 1) return null
  const pieces = body.split(foreign[0])
  if (pieces.length !== 2 || !/^\d*$/.test(pieces[0]) || !/^\d+$/.test(pieces[1]) || pieces[1].length === 3) return null
  return result(pieces[0], pieces[1])
}

// A stored decimal shown for editing in the UI language, digit for digit:
// going through a number would round long fractions.
export function decimalInput(value: string, locale: string): string {
  return value.replace('.', separators(locale).decimal)
}
