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

  it('reads a Windows-1251 file as Cyrillic whatever the browser language', async () => {
    vi.stubGlobal('navigator', { language: 'en-US' })
    // Дата;Сумма;Описание / 1;2;Пятёрочка
    const bytes = new Uint8Array([
      0xc4, 0xe0, 0xf2, 0xe0, 0x3b, 0xd1, 0xf3, 0xec, 0xec, 0xe0, 0x3b, 0xce, 0xef, 0xe8, 0xf1, 0xe0, 0xed, 0xe8, 0xe5, 0x0a,
      0x31, 0x3b, 0x32, 0x3b, 0xcf, 0xff, 0xf2, 0xb8, 0xf0, 0xee, 0xf7, 0xea, 0xe0, 0x0a,
    ])
    const r = await readCsvFile(new File([bytes], 'x.csv'))
    expect(r).toMatchObject({ header: ['Дата', 'Сумма', 'Описание'], rows: [['1', '2', 'Пятёрочка']] })
  })

  it('reads a Windows-1252 file as Latin whatever the browser language', async () => {
    vi.stubGlobal('navigator', { language: 'ru-RU' })
    // Müller,Straße / 1,Café
    const bytes = new Uint8Array([0x4d, 0xfc, 0x6c, 0x6c, 0x65, 0x72, 0x2c, 0x53, 0x74, 0x72, 0x61, 0xdf, 0x65, 0x0a, 0x31, 0x2c, 0x43, 0x61, 0x66, 0xe9, 0x0a])
    const r = await readCsvFile(new File([bytes], 'x.csv'))
    expect(r).toMatchObject({ header: ['Müller', 'Straße'], rows: [['1', 'Café']] })
  })

  it('leaves a file with nothing to tell the encodings apart to the browser language', async () => {
    // "\xe9 a\xe9": one word that reads as Cyrillic, one as an accented Latin word
    const bytes = new Uint8Array([0xe9, 0x20, 0x61, 0xe9, 0x2c, 0x62, 0x0a, 0x31, 0x2c, 0x32, 0x0a])
    vi.stubGlobal('navigator', { language: 'ru-RU' })
    expect(await readCsvFile(new File([bytes], 'x.csv'))).toMatchObject({ header: ['й aй', 'b'] })
    vi.stubGlobal('navigator', { language: 'de-DE' })
    expect(await readCsvFile(new File([bytes], 'x.csv'))).toMatchObject({ header: ['é aé', 'b'] })
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
