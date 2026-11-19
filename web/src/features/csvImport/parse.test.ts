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
    expect(p.rowLines).toEqual([2, 4])
  })

  it('skips blank lines and handles a missing trailing newline', () => {
    const p = parseCsvText('a,b\n\n1,2\n\n3,4')
    expect(p.rows).toEqual([['1', '2'], ['3', '4']])
    expect(p.rawLines).toEqual(['1,2', '3,4'])
    expect(p.rowLines).toEqual([3, 5])
  })

  it('counts CRLF and lone CR line ends, inside quoted cells too', () => {
    expect(parseCsvText('a,b\r\n\r\n"x\r\ny",1\r\n2,3\r\n').rowLines).toEqual([3, 5])
    expect(parseCsvText('a,b\r"x\ry",1\r2,3').rowLines).toEqual([2, 4])
  })

  it('counts delimiters after an inch mark when picking the delimiter', () => {
    const p = parseCsvText('id,a,b,c\na;1,TV 55" x,2,3\nb;2,3,TV 27" x,4\nc;3,4,5,TV 32" x\nd;4,TV 40" y,5,6\n')
    expect(p.delimiter).toBe(',')
    expect(p.rows[0]).toEqual(['a;1', 'TV 55" x', '2', '3'])
  })

  it('keeps a quote inside an unquoted cell as text', () => {
    const p = parseCsvText('a,b,c\n1,TV 55" wide,2\n3,4,5\n6,7,8\n')
    expect(p.rows).toEqual([['1', 'TV 55" wide', '2'], ['3', '4', '5'], ['6', '7', '8']])
  })
})

type Encoding = 'windows-1251' | 'windows-1252'

// the bytes a legacy encoding writes for text, built from the browser's own decoder
function encode(text: string, encoding: Encoding): Uint8Array<ArrayBuffer> {
  const decoder = new TextDecoder(encoding)
  const table = new Map<string, number>()
  for (let byte = 0x80; byte <= 0xff; byte++) table.set(decoder.decode(new Uint8Array([byte])), byte)
  return new Uint8Array(
    [...text].map((char) => {
      const byte = char.charCodeAt(0) < 0x80 ? char.charCodeAt(0) : table.get(char)
      if (byte === undefined) throw new Error(`${char} is not in ${encoding}`)
      return byte
    }),
  )
}

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
    // "\xe9,b": a lone letter, "й" in one reading and "é" in the other
    const bytes = new Uint8Array([0xe9, 0x2c, 0x62, 0x0a, 0x31, 0x2c, 0x32, 0x0a])
    vi.stubGlobal('navigator', { language: 'ru-RU' })
    expect(await readCsvFile(new File([bytes], 'x.csv'))).toMatchObject({ header: ['й', 'b'] })
    vi.stubGlobal('navigator', { language: 'de-DE' })
    expect(await readCsvFile(new File([bytes], 'x.csv'))).toMatchObject({ header: ['é', 'b'] })
  })

  describe.each(['en-US', 'ru-RU'])('with the browser in %s', (language) => {
    const files: [string, Encoding, string][] = [
      ['German with umlauts and euro signs', 'windows-1252', 'Datum;Betrag;Empfänger\n01.02.2024;-12,50 €;Bäckerei Müller\n02.02.2024;-3 €;Straße Größe\n03.02.2024;-7 €;Café\n'],
      ['German statement with more euro signs than accented words', 'windows-1252', 'Betrag;Name\n-12 €;Müller\n-3 €;Bank\n-7 €;Bank\n'],
      ['French with accents', 'windows-1252', 'Date;Montant;Libellé\n01/02/2024;-12,50;Café à emporter\n02/02/2024;-3;Prélèvement été\n03/02/2024;-7;Boulangerie à côté\n'],
      ['Italian with a lone è and a euro sign', 'windows-1252', 'Data;Importo;Descrizione\n01/02/2024;-12,50 €;è pagato\n02/02/2024;-3 €;è\n'],
      ['Russian', 'windows-1251', 'Дата;Сумма;Описание\n01.02.2024;-12,50;Пятёрочка у дома\n02.02.2024;-3;Перевод в банк\n'],
      ['Ukrainian', 'windows-1251', 'Дата;Сума;Опис\n01.02.2024;-12,50;Їжа і кава\n02.02.2024;-3;Єдиний внесок і податок\n'],
    ]
    it.each(files)('reads a %s file as %s', async (_, encoding, text) => {
      vi.stubGlobal('navigator', { language })
      const r = await readCsvFile(new File([encode(text, encoding)], 'x.csv'))
      expect(r).toEqual(parseCsvText(text))
    })
  })

  // the unclosed quote would fold every later row into one cell
  it('reports a file whose quoted cell is never closed as unreadable', async () => {
    expect(await readCsvFile(new File(['a,b\n1,"open\n2,3\n4,5\n'], 'x.csv'))).toEqual({ error: 'unreadable' })
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
