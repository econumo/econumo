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

// A quote opens a quoted cell only at the start of a cell (`55" TV` keeps its quote);
// `unterminated` reports a quoted cell still open at the end of the text.
function splitRecords(text: string, delimiter: string): { records: RawRecord[]; unterminated: boolean } {
  const records: RawRecord[] = []
  let cells: string[] = []
  let current = ''
  let cellStarted = false
  let inQuotes = false
  let start = 0

  const endCell = () => {
    cells.push(current)
    current = ''
    cellStarted = false
  }
  const finish = (end: number) => {
    endCell()
    const raw = text.slice(start, end)
    if (cells.length > 1 || cells[0].trim() !== '') {
      records.push({ cells, raw })
    }
    cells = []
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
    } else if (char === '"' && !cellStarted) {
      inQuotes = true
      cellStarted = true
    } else if (char === delimiter) {
      endCell()
    } else if (char === '\n' || char === '\r') {
      finish(i)
      if (char === '\r' && text[i + 1] === '\n') i++
      start = i + 1
    } else {
      current += char
      cellStarted = true
    }
  }
  if (start < text.length || current !== '' || cells.length > 0) finish(text.length)
  return { records, unterminated: inQuotes }
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

function parse(text: string): { file: ParsedFile; unterminated: boolean } {
  const clean = text.replace(/^\uFEFF/, '')
  const delimiter = sniffDelimiter(clean)
  const { records, unterminated } = splitRecords(clean, delimiter)
  if (records.length === 0) {
    return { file: { header: [], rows: [], delimiter, rawLines: [] }, unterminated }
  }
  return {
    file: {
      header: uniqueNames(records[0].cells),
      rows: records.slice(1).map((r) => r.cells),
      delimiter,
      rawLines: records.slice(1).map((r) => r.raw),
    },
    unterminated,
  }
}

export function parseCsvText(text: string): ParsedFile {
  return parse(text).file
}

const CYRILLIC_LANGUAGES = ['ru', 'uk', 'be', 'bg', 'sr']

// Letters Windows-1251 makes of bytes that Windows-1252 uses for symbols (€ Š Œ Ž š œ ž Ÿ
// £ ¼ ½ ¾ ƒ) and that Russian, Ukrainian and Belarusian never write on their own.
const SYMBOL_LETTERS = /^[ЂЃѓЉЊЌЋЏђљњќћџЈјЅѕ]$/u

// Both readings are scored on the words they make, read as Windows-1251:
// - a word of two or more Cyrillic letters is how Russian or Ukrainian text reads;
// - a Cyrillic letter inside a Latin word ("Müller" -> "Mьller", "Café" -> "Cafй") is an
//   accented Latin letter, and a lone letter from a symbol byte ("12 €" -> "12 Ђ") is a
//   Windows-1252 symbol.
// One-letter Cyrillic words ("и", "в", "і", or a lone "à") count for neither. The browser
// language only breaks a tie.
function legacyEncoding(bytes: Uint8Array): 'windows-1251' | 'windows-1252' {
  let cyrillic = 0
  let latin = 0
  for (const [word] of new TextDecoder('windows-1251').decode(bytes).matchAll(/\p{L}+/gu)) {
    if (!/\p{Script=Cyrillic}/u.test(word)) continue
    if (/[A-Za-z]/.test(word) || SYMBOL_LETTERS.test(word)) latin++
    else if (word.length >= 2) cyrillic++
  }
  if (cyrillic !== latin) return cyrillic > latin ? 'windows-1251' : 'windows-1252'
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
  // an unclosed quote swallows every row after it into one cell, so nothing below it can be trusted
  const { file: parsed, unterminated } = parse(text)
  if (unterminated) return { error: 'unreadable' }
  if (parsed.header.length === 0 || parsed.rows.length === 0) return { error: 'empty' }
  return parsed
}
