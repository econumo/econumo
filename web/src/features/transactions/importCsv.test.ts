import { analyzeCsv, autoDetect, buildImportPayload, defaultSelection, FIELD_KEYS, runImport, selectionValid } from './importCsv'
import type { ImportSelection } from './importCsv'

const LABELS = {
  account: 'Account',
  date: 'Date',
  amount: 'Amount',
  amountInflow: 'Amount (Inflow)',
  amountOutflow: 'Amount (Outflow)',
  category: 'Category',
  description: 'Description',
  payee: 'Payee',
  tag: 'Tag',
}

it('analyzeCsv collects first non-empty samples, truncated at 25 chars', () => {
  const text = 'Account,Date,Note\nCash,2026-01-02,\nBank,2026-01-03,a very very long description indeed\n'
  const analysis = analyzeCsv(text)
  expect(analysis.header).toEqual(['Account', 'Date', 'Note'])
  expect(analysis.rows).toHaveLength(2)
  expect(analysis.samples.Account).toBe('Cash')
  expect(analysis.samples.Note).toBe('a very very long descript…')
})

it('autoDetect maps matching headers and stays in single amount mode', () => {
  const detected = autoDetect(['Account', 'Date', 'Amount', 'Category', 'Description', 'Payee'], LABELS)
  expect(detected.columns).toMatchObject({
    account: 'Account', date: 'Date', amount: 'Amount', category: 'Category', description: 'Description', payee: 'Payee',
  })
  expect(detected.amountMode).toBe('single')
})

it('autoDetect flips to dual mode when inflow+outflow columns match', () => {
  const detected = autoDetect(['Account', 'Date', 'In', 'Out'], LABELS)
  expect(detected.columns.amountInflow).toBe('In')
  expect(detected.columns.amountOutflow).toBe('Out')
  expect(detected.amountMode).toBe('dual')
})

function validBase(): ImportSelection {
  const sel = defaultSelection()
  sel.columns.account = 'Account'
  sel.columns.date = 'Date'
  sel.columns.amount = 'Amount'
  return sel
}

it('selectionValid enforces account, date, and amount per mode', () => {
  expect(selectionValid(defaultSelection())).toBe(false)
  expect(selectionValid(validBase())).toBe(true)

  const fixedAccount = validBase()
  fixedAccount.columns.account = null
  fixedAccount.modes.account = 'existing'
  expect(selectionValid(fixedAccount)).toBe(false)
  fixedAccount.fixed.accountId = 'a1'
  expect(selectionValid(fixedAccount)).toBe(true)

  const manualDate = validBase()
  manualDate.columns.date = null
  manualDate.modes.date = 'manual'
  manualDate.fixed.date = '2026-13-99'
  expect(selectionValid(manualDate)).toBe(false)
  manualDate.fixed.date = '2026-05-01'
  expect(selectionValid(manualDate)).toBe(true)

  const dual = validBase()
  dual.amountMode = 'dual'
  dual.columns.amount = null
  expect(selectionValid(dual)).toBe(false)
  dual.columns.amountInflow = 'In'
  dual.columns.amountOutflow = 'Out'
  expect(selectionValid(dual)).toBe(true)
})

it('buildImportPayload always emits all 9 mapping keys and only truthy fixed fields', () => {
  const sel = validBase()
  sel.modes.category = 'existing'
  sel.fixed.categoryId = 'cat-1'
  sel.modes.description = 'manual'
  sel.fixed.description = '  bulk import  '
  sel.columns.payee = 'Payee'
  const { mapping, fields } = buildImportPayload(sel)
  expect(Object.keys(mapping).sort()).toEqual([...FIELD_KEYS].sort())
  expect(mapping).toMatchObject({ account: 'Account', date: 'Date', amount: 'Amount', payee: 'Payee', category: null, description: null, tag: null })
  expect(mapping.amountInflow).toBeNull()
  expect(mapping.amountOutflow).toBeNull()
  expect(fields).toEqual({ categoryId: 'cat-1', description: 'bulk import' })
})

it('buildImportPayload in dual mode nulls the single amount column', () => {
  const sel = validBase()
  sel.amountMode = 'dual'
  sel.columns.amountInflow = 'In'
  sel.columns.amountOutflow = 'Out'
  const { mapping } = buildImportPayload(sel)
  expect(mapping.amount).toBeNull()
  expect(mapping.amountInflow).toBe('In')
  expect(mapping.amountOutflow).toBe('Out')
})

it('runImport chunks at 500 rows, remaps error rows, and tolerates chunk failures', async () => {
  const rows = Array.from({ length: 1200 }, (_, i) => ['Cash', '2026-01-02', String(i + 1)])
  const analysis = { header: ['Account', 'Date', 'Amount'], rows, samples: {} }
  const files: { name: string; rows: number }[] = []
  const post = vi.fn(async (file: File): Promise<{ imported: number; skipped: number; errors: Record<string, number[]> }> => {
    files.push({ name: file.name, rows: (await file.text()).trim().split('\n').length - 1 })
    if (files.length === 2) {
      throw Object.assign(new Error('Request failed with status code 400'), {
        isAxiosError: true,
        response: { data: { success: false, message: 'boom', code: 400, errors: {} } },
      })
    }
    if (files.length === 3) {
      return { imported: 199, skipped: 1, errors: { "Invalid date format 'x'": [3] } }
    }
    return { imported: 500, skipped: 0, errors: {} }
  })
  const progress: [number, number][] = []
  const result = await runImport(analysis, validBase(), post, (done, total) => progress.push([done, total]))
  expect(files).toEqual([
    { name: 'chunk_0.csv', rows: 500 },
    { name: 'chunk_1.csv', rows: 500 },
    { name: 'chunk_2.csv', rows: 200 },
  ])
  expect(result.imported).toBe(699)
  expect(result.failed).toBe(501) // 500 rows of the failed chunk + 1 skipped
  expect(result.errors).toEqual([
    { message: 'Chunk 2 failed: boom', rows: [] },
    { message: "Invalid date format 'x'", rows: [1003] }, // chunk 2 row 3 -> file row 1003
  ])
  expect(progress).toEqual([[1, 3], [2, 3], [3, 3]])
})
