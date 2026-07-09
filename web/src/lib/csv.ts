// Quote-aware CSV helpers for the import/export flows (port of the Vue
// ImportCsvModal parser; the backend tolerates ragged rows, so we do too).

export function parseCsvLine(line: string): string[] {
  const cells: string[] = []
  let current = ''
  let inQuotes = false
  for (let i = 0; i < line.length; i++) {
    const char = line[i]
    if (inQuotes) {
      if (char === '"') {
        if (line[i + 1] === '"') {
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
    } else if (char === ',') {
      cells.push(current)
      current = ''
    } else {
      current += char
    }
  }
  cells.push(current)
  return cells
}

export function parseCsv(text: string): { header: string[]; rows: string[][] } {
  const clean = text.replace(/^﻿/, '')
  const lines = clean.split(/\r?\n/).filter((line) => line.trim() !== '')
  if (lines.length === 0) {
    return { header: [], rows: [] }
  }
  return { header: parseCsvLine(lines[0]), rows: lines.slice(1).map(parseCsvLine) }
}

function serializeCell(cell: string): string {
  if (/[",\n\r]/.test(cell)) {
    return `"${cell.replaceAll('"', '""')}"`
  }
  return cell
}

export function buildCsvText(header: string[], rows: string[][]): string {
  return [header, ...rows].map((row) => row.map(serializeCell).join(',')).join('\n') + '\n'
}

export function chunkRows<T>(rows: T[], size: number): T[][] {
  const chunks: T[][] = []
  for (let i = 0; i < rows.length; i += size) {
    chunks.push(rows.slice(i, i + size))
  }
  return chunks
}

// Backend row numbers are 1-based counting the header (data row N = N+1); row 0
// marks a top-level error and must stay 0 across chunks.
export function remapRow(chunkRow: number, chunkIndex: number, chunkSize: number): number {
  return chunkRow === 0 ? 0 : chunkRow + chunkIndex * chunkSize
}
