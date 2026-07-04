import { buildCsvText, chunkRows, parseCsv, parseCsvLine, remapRow } from './csv'

it('parseCsvLine handles quotes, escaped quotes, and commas in cells', () => {
  expect(parseCsvLine('a,b,c')).toEqual(['a', 'b', 'c'])
  expect(parseCsvLine('"a,x",b')).toEqual(['a,x', 'b'])
  expect(parseCsvLine('"he said ""hi""",2')).toEqual(['he said "hi"', '2'])
  expect(parseCsvLine('a,,c')).toEqual(['a', '', 'c'])
})

it('parseCsv strips BOM and blank lines', () => {
  const { header, rows } = parseCsv('﻿Date,Amount\n2026-01-02,-5\n\n2026-01-03,7\n')
  expect(header).toEqual(['Date', 'Amount'])
  expect(rows).toEqual([
    ['2026-01-02', '-5'],
    ['2026-01-03', '7'],
  ])
})

it('parseCsv handles CRLF line endings', () => {
  const { header, rows } = parseCsv('a,b\r\n1,2\r\n')
  expect(header).toEqual(['a', 'b'])
  expect(rows).toEqual([['1', '2']])
})

it('buildCsvText round-trips through parseCsv', () => {
  const text = buildCsvText(['a', 'b'], [['x,1', 'he said "hi"'], ['plain', '2']])
  expect(parseCsv(text)).toEqual({ header: ['a', 'b'], rows: [['x,1', 'he said "hi"'], ['plain', '2']] })
})

it('chunkRows splits at the boundary; remapRow restores original file rows', () => {
  expect(chunkRows([1, 2, 3, 4, 5], 2)).toEqual([[1, 2], [3, 4], [5]])
  expect(remapRow(3, 0, 500)).toBe(3) // chunk 0: unchanged
  expect(remapRow(2, 1, 500)).toBe(502) // chunk 1 data row 1 = file row 502 (header-inclusive numbering)
  expect(remapRow(0, 4, 500)).toBe(0) // top-level errors stay 0
})
