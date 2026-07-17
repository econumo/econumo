const pad = (n: number) => String(n).padStart(2, '0')

export function formatDateTime(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

export function formatDate(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

export function parseDateTime(s: string): Date {
  const [datePart, timePart = '00:00:00'] = s.split(' ')
  const [y, m, d] = datePart.split('-').map(Number)
  const [hh, mm, ss] = timePart.split(':').map(Number)
  return new Date(y, m - 1, d, hh, mm, ss)
}

export function dayKey(s: string): string {
  return s.split(' ')[0]
}

function ordinal(day: number): string {
  const mod100 = day % 100
  if (mod100 >= 11 && mod100 <= 13) {
    return `${day}th`
  }
  switch (day % 10) {
    case 1: return `${day}st`
    case 2: return `${day}nd`
    case 3: return `${day}rd`
    default: return `${day}th`
  }
}

export function formatDayHeading(day: string, lang = 'en'): string {
  const d = parseDateTime(day)
  // the "1st July 2026" ordinal style only exists in English; other languages
  // get their locale's own full-date form (ru: "1 июля 2026 г.")
  if (lang !== 'en') {
    return new Intl.DateTimeFormat(lang, { day: 'numeric', month: 'long', year: 'numeric' }).format(d)
  }
  const month = new Intl.DateTimeFormat('en', { month: 'long' }).format(d)
  return `${ordinal(d.getDate())} ${month} ${d.getFullYear()}`
}

export function isToday(day: string): boolean {
  return day === formatDate(new Date())
}

export function isYesterday(day: string): boolean {
  const y = new Date()
  y.setDate(y.getDate() - 1)
  return day === formatDate(y)
}

export function isFuture(dateTime: string): boolean {
  const tomorrow = new Date()
  tomorrow.setHours(0, 0, 0, 0)
  tomorrow.setDate(tomorrow.getDate() + 1)
  return parseDateTime(dateTime).getTime() >= tomorrow.getTime()
}
