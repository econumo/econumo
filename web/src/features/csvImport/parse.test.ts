import { afterEach, describe, expect, it, vi } from 'vitest'
import { MAX_FILE_SIZE, parseCsvText, readCsvFile } from './parse'

describe('parseCsvText', () => {
  it('strips BOM and CRLF and detects a semicolon delimiter', () => {
    const p = parseCsvText('\uFEFFDatum;Betrag\r\n01.02.2026;-4,50\r\n')
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
  afterEach(() => vi.unstubAllGlobals())

  it('names blank headers by position', () => {
    expect(parseCsvText('a,,\n1,2,3\n').header).toEqual(['a', 'Column 2', 'Column 3'])
    expect(parseCsvText(',Column 1\n1,2\n').header).toEqual(['Column 1', 'Column 1 (2)'])
  })

  it('falls back to windows-1251 for Cyrillic locales', async () => {
    vi.stubGlobal('navigator', { language: 'ru-RU' })
    const bytes = new Uint8Array([0xc4, 0xe0, 0xf2, 0xe0, 0x3b, 0xd1, 0xf3, 0xec, 0xec, 0xe0, 0x0a, 0x31, 0x3b, 0x32, 0x0a])
    const r = await readCsvFile(new File([bytes], 'x.csv'))
    expect(r).toMatchObject({ header: ['Дата', 'Сумма'] })
  })

  it('falls back to windows-1252 otherwise', async () => {
    vi.stubGlobal('navigator', { language: 'de-DE' })
    const bytes = new Uint8Array([0x4d, 0xfc, 0x6c, 0x6c, 0x65, 0x72, 0x2c, 0x62, 0x0a, 0x31, 0x2c, 0x32, 0x0a])
    const r = await readCsvFile(new File([bytes], 'x.csv'))
    expect(r).toMatchObject({ header: ['Müller', 'b'] })
  })

  it('reads UTF-8 with a BOM', async () => {
    const r = await readCsvFile(new File(['\uFEFFДата;Сумма\n1;2\n'], 'x.csv'))
    expect(r).toMatchObject({ header: ['Дата', 'Сумма'] })
  })

  it('reports unreadable when the file cannot be read', async () => {
    const broken = { size: 10, arrayBuffer: () => Promise.reject(new Error('boom')) } as unknown as File
    expect(await readCsvFile(broken)).toEqual({ error: 'unreadable' })
    expect(await readCsvFile(new File([new Uint8Array([0x61, 0, 0x62])], 'x.csv'))).toEqual({ error: 'unreadable' })
  })

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
