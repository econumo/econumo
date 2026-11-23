export type DateOrder = 'ymd' | 'dmy' | 'mdy'

export interface ParsedDate {
  date: string
  time: string | null
}

const TIME = String.raw`(?:[ T](\d{1,2}):(\d{2})(?::(\d{2})(?:\.\d+)?)?(?:Z|[+-]\d{2}:?\d{2})?)?`
const YEAR_FIRST = new RegExp(String.raw`^(\d{4})[-/.](\d{1,2})[-/.](\d{1,2})${TIME}$`)
const YEAR_LAST = new RegExp(String.raw`^(\d{1,2})[-/.](\d{1,2})[-/.](\d{4})${TIME}$`)

const MDY_LOCALES = new Set(['en-us', 'en-ph', 'es-us'])

function clean(value: string): string {
  return value.replace(/^[\s"']+|[\s"']+$/g, '')
}

const pad = (n: number, width = 2) => String(n).padStart(width, '0')

function build(y: number, m: number, d: number, hh?: string, mm?: string, ss?: string): ParsedDate | null {
  const probe = new Date(Date.UTC(y, m - 1, d))
  probe.setUTCFullYear(y)
  if (probe.getUTCFullYear() !== y || probe.getUTCMonth() !== m - 1 || probe.getUTCDate() !== d) return null
  let time: string | null = null
  if (hh !== undefined && mm !== undefined) {
    const h = Number(hh)
    const mi = Number(mm)
    const s = ss === undefined ? 0 : Number(ss)
    if (h > 23 || mi > 59 || s > 59) return null
    time = `${pad(h)}:${pad(mi)}:${pad(s)}`
  }
  return { date: `${pad(y, 4)}-${pad(m)}-${pad(d)}`, time }
}

export function parseDateCell(value: string, order: DateOrder): ParsedDate | null {
  const s = clean(value)
  if (s === '') return null

  if (/^\d+$/.test(s)) {
    if (s.length === 8) return build(Number(s.slice(0, 4)), Number(s.slice(4, 6)), Number(s.slice(6)))
    if (s.length === 10 || s.length === 13) {
      const t = new Date(s.length === 10 ? Number(s) * 1000 : Number(s))
      return build(
        t.getUTCFullYear(), t.getUTCMonth() + 1, t.getUTCDate(),
        String(t.getUTCHours()), String(t.getUTCMinutes()), String(t.getUTCSeconds()),
      )
    }
    return null
  }

  let m = YEAR_FIRST.exec(s)
  if (m) return build(Number(m[1]), Number(m[2]), Number(m[3]), m[4], m[5], m[6])

  m = YEAR_LAST.exec(s)
  if (m) {
    const [a, b] = [Number(m[1]), Number(m[2])]
    const [day, month] = order === 'mdy' ? [b, a] : [a, b]
    return build(Number(m[3]), month, day, m[4], m[5], m[6])
  }
  return null
}

export function localeDateOrder(locale: string): 'dmy' | 'mdy' {
  return MDY_LOCALES.has(locale.replace('_', '-').toLowerCase()) ? 'mdy' : 'dmy'
}

export function detectDateOrder(
  values: string[],
  locale: string,
): { order: DateOrder; ambiguous: boolean; example: string | null } {
  const fallback = localeDateOrder(locale)
  let firstOver12 = false
  let secondOver12 = false
  let example: string | null = null

  for (const raw of values) {
    const s = clean(raw)
    if (YEAR_FIRST.test(s)) return { order: 'ymd', ambiguous: false, example: null }
    const m = YEAR_LAST.exec(s)
    if (!m) continue
    example ??= s
    if (Number(m[1]) > 12) firstOver12 = true
    if (Number(m[2]) > 12) secondOver12 = true
  }

  if (firstOver12) return { order: 'dmy', ambiguous: false, example: null }
  if (secondOver12) return { order: 'mdy', ambiguous: false, example: null }
  // month-first dates are written with slashes or dashes, never dots: 03.11.2026 is 3 November anywhere
  const order = example !== null && example.includes('.') ? 'dmy' : fallback
  return { order, ambiguous: example !== null, example }
}
