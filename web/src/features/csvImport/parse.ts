export type CsvDelimiter = ',' | ';' | '\t' | '|'

export interface ParsedFile {
  header: string[]
  rows: string[][]
  delimiter: CsvDelimiter
  rawLines: string[]
}

export const MAX_FILE_SIZE = 10485760

const DELIMITERS: CsvDelimiter[] = [',', ';', '\t', '|']
const SNIFF_LINES = 20

interface RawRecord {
  cells: string[]
  raw: string
}

function splitRecords(text: string, delimiter: string): RawRecord[] {
  const records: RawRecord[] = []
  let cells: string[] = []
  let current = ''
  let inQuotes = false
  let start = 0

  const finish = (end: number) => {
    cells.push(current)
    const raw = text.slice(start, end)
    if (cells.length > 1 || cells[0].trim() !== '') {
      records.push({ cells, raw })
    }
    cells = []
    current = ''
  }

  for (let i = 0; i < text.length; i++) {
    const char = text[i]
    if (inQuotes) {
      if (char === '"') {
        if (text[i + 1] === '"') {
          current += '"'
          i++
        } else {
          inQuotes = false
        }
      } else {
        current += char
      }
    } else if (char === '"') {
      inQuotes = true
    } else if (char === delimiter) {
      cells.push(current)
      current = ''
    } else if (char === '\n' || char === '\r') {
      finish(i)
      if (char === '\r' && text[i + 1] === '\n') i++
      start = i + 1
    } else {
      current += char
    }
  }
  if (start < text.length || current !== '' || cells.length > 0) finish(text.length)
  return records
}

function sniffDelimiter(text: string): CsvDelimiter {
  const lines = text
    .split(/\r?\n/)
    .filter((l) => l.trim() !== '')
    .slice(0, SNIFF_LINES)
  let best: CsvDelimiter = ','
  let bestScore = 0
  let bestCount = 0
  for (const candidate of DELIMITERS) {
    const counts = new Map<number, number>()
    for (const line of lines) {
      let n = 0
      let inQuotes = false
      for (const char of line) {
        if (char === '"') inQuotes = !inQuotes
        else if (!inQuotes && char === candidate) n++
      }
      if (n > 0) counts.set(n, (counts.get(n) ?? 0) + 1)
    }
    for (const [count, lineCount] of counts) {
      if (lineCount > bestScore || (lineCount === bestScore && count > bestCount)) {
        best = candidate
        bestScore = lineCount
        bestCount = count
      }
    }
  }
  return best
}

function uniqueNames(names: string[]): string[] {
  const used = new Set<string>()
  const seen = new Map<string, number>()
  return names.map((raw, index) => {
    const name = raw.trim() === '' ? `Column ${index + 1}` : raw.trim()
    let n = seen.get(name) ?? 0
    let candidate = name
    do {
      n++
      candidate = n === 1 ? name : `${name} (${n})`
    } while (used.has(candidate))
    seen.set(name, n)
    used.add(candidate)
    return candidate
  })
}

export function parseCsvText(text: string): ParsedFile {
  const clean = text.replace(/^\uFEFF/, '')
  const delimiter = sniffDelimiter(clean)
  const records = splitRecords(clean, delimiter)
  if (records.length === 0) {
    return { header: [], rows: [], delimiter, rawLines: [] }
  }
  return {
    header: uniqueNames(records[0].cells),
    rows: records.slice(1).map((r) => r.cells),
    delimiter,
    rawLines: records.slice(1).map((r) => r.raw),
  }
}

const CYRILLIC_LANGUAGES = ['ru', 'uk', 'be', 'bg', 'sr']

// Read as Windows-1251, a Russian word is all Cyrillic letters, while a Latin word with an
// accent ("Müller" -> "Mьller") mixes Cyrillic and ASCII letters; whichever kind of word is
// more common decides. The browser language only breaks a tie.
function legacyEncoding(bytes: Uint8Array): 'windows-1251' | 'windows-1252' {
  let cyrillicWords = 0
  let mixedWords = 0
  for (const [word] of new TextDecoder('windows-1251').decode(bytes).matchAll(/\p{L}+/gu)) {
    const cyrillic = /\p{Script=Cyrillic}/u.test(word)
    if (!cyrillic) continue
    if (/[A-Za-z]/.test(word)) mixedWords++
    else cyrillicWords++
  }
  if (cyrillicWords !== mixedWords) return cyrillicWords > mixedWords ? 'windows-1251' : 'windows-1252'
  const language = (typeof navigator === 'undefined' ? '' : navigator.language).toLowerCase()
  return CYRILLIC_LANGUAGES.some((l) => language.startsWith(l)) ? 'windows-1251' : 'windows-1252'
}

function decodeBytes(bytes: Uint8Array): string {
  try {
    return new TextDecoder('utf-8', { fatal: true }).decode(bytes)
  } catch {
    return new TextDecoder(legacyEncoding(bytes)).decode(bytes)
  }
}

export async function readCsvFile(
  file: File,
): Promise<ParsedFile | { error: 'too_big' | 'empty' | 'unreadable' }> {
  if (file.size > MAX_FILE_SIZE) return { error: 'too_big' }
  let text: string
  try {
    text = decodeBytes(new Uint8Array(await file.arrayBuffer()))
  } catch {
    return { error: 'unreadable' }
  }
  if (text.includes('\u0000')) return { error: 'unreadable' }
  const parsed = parseCsvText(text)
  if (parsed.header.length === 0 || parsed.rows.length === 0) return { error: 'empty' }
  return parsed
}
