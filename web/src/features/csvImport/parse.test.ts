import { describe, expect, it } from 'vitest'
import { MAX_FILE_SIZE, parseCsvText, readCsvFile } from './parse'

describe('parseCsvText', () => {
  it('strips BOM and CRLF and detects a semicolon delimiter', () => {
    const p = parseCsvText('﻿Datum;Betrag\r\n01.02.2026;-4,50\r\n')
    expect(p.header).toEqual(['Datum', 'Betrag'])
    expect(p.delimiter).toBe(';')
    expect(p.rows).toEqual([['01.02.2026', '-4,50']])
    expect(p.rawLines).toEqual(['01.02.2026;-4,50'])
  })

  it('keeps quoted delimiters in one cell', () => {
    const p = parseCsvText('a,b\n"x, y",2\n')
    expect(p.delimiter).toBe(',')
    expect(p.rows).toEqual([['x, y', '2']])
  })

  it('detects tab and pipe', () => {
    expect(parseCsvText('a\tb\n1\t2\n').delimiter).toBe('\t')
    expect(parseCsvText('a|b\n1|2\n').delimiter).toBe('|')
  })

  it('disambiguates duplicate headers', () => {
    expect(parseCsvText('Amount,Amount,Amount\n1,2,3\n').header).toEqual(['Amount', 'Amount (2)', 'Amount (3)'])
  })

  it('keeps multi-line quoted cells and their original raw text', () => {
    const p = parseCsvText('a,b\n"line1\nline2",2\n3,4\n')
    expect(p.rows).toEqual([['line1\nline2', '2'], ['3', '4']])
    expect(p.rawLines).toEqual(['"line1\nline2",2', '3,4'])
  })

  it('skips blank lines and handles a missing trailing newline', () => {
    const p = parseCsvText('a,b\n\n1,2\n\n3,4')
    expect(p.rows).toEqual([['1', '2'], ['3', '4']])
    expect(p.rawLines).toEqual(['1,2', '3,4'])
  })
})

describe('readCsvFile', () => {
  it('reads a file', async () => {
    const r = await readCsvFile(new File(['a,b\n1,2\n'], 'x.csv'))
    expect('error' in r).toBe(false)
  })
  it('reports empty', async () => {
    expect(await readCsvFile(new File([''], 'x.csv'))).toEqual({ error: 'empty' })
    expect(await readCsvFile(new File(['a,b\n'], 'x.csv'))).toEqual({ error: 'empty' })
  })
  it('accepts exactly MAX_FILE_SIZE bytes and rejects one more', async () => {
    const head = 'a,b\n1,2\n'
    const sized = (n: number) => new File([head + ' '.repeat(n - head.length)], 'x.csv')
    expect(MAX_FILE_SIZE).toBe(10485760)
    const ok = await readCsvFile(sized(MAX_FILE_SIZE))
    expect('error' in ok).toBe(false)
    expect(await readCsvFile(sized(MAX_FILE_SIZE + 1))).toEqual({ error: 'too_big' })
  })
})
