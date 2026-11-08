import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { fixtureAccounts } from '@/test/fixtures'
import type { AccountDto } from '@/api/dto/account'
import { runImportKeepingBalance, targetAccountIds, analyzeCsv, autoDetect, buildImportPayload, countNewLabels, defaultSelection, FIELD_KEYS, runImport, selectionValid } from './importCsv'
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
  labels: 'Labels',
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

it('buildImportPayload always emits all 10 mapping keys and only truthy fixed fields', () => {
  const sel = validBase()
  sel.modes.category = 'existing'
  sel.fixed.categoryId = 'cat-1'
  sel.modes.description = 'manual'
  sel.fixed.description = '  bulk import  '
  sel.columns.payee = 'Payee'
  const { mapping, fields } = buildImportPayload(sel)
  expect(Object.keys(mapping).sort()).toEqual([...FIELD_KEYS].sort())
  expect(mapping).toMatchObject({ account: 'Account', date: 'Date', amount: 'Amount', payee: 'Payee', category: null, description: null, tag: null, labels: null })
  expect(mapping.amountInflow).toBeNull()
  expect(mapping.amountOutflow).toBeNull()
  expect(fields).toEqual({ categoryId: 'cat-1', description: 'bulk import', labelsSeparator: ';' })
})

it('includes labels in the mapping payload', () => {
  const mapped = validBase()
  mapped.modes.labels = 'csv_column'
  mapped.columns.labels = 'Labels'
  expect(buildImportPayload(mapped).mapping.labels).toBe('Labels')

  const unmapped = validBase()
  unmapped.modes.labels = 'csv_column'
  unmapped.columns.labels = null
  expect(buildImportPayload(unmapped).mapping.labels).toBeNull()
})

it('sends the chosen separator, defaulting to ";"', () => {
  const sel = validBase()
  expect(buildImportPayload(sel).fields.labelsSeparator).toBe(';')

  sel.labelsSeparator = '|'
  expect(buildImportPayload(sel).fields.labelsSeparator).toBe('|')
})

it('sends the labelIds override in existing mode, comma-joined', () => {
  const sel = validBase()
  sel.modes.labels = 'existing'
  sel.fixed.labelIds = ['label-1', 'label-2']
  expect(buildImportPayload(sel).fields.labelIds).toBe('label-1,label-2')

  sel.fixed.labelIds = []
  expect(buildImportPayload(sel).fields.labelIds).toBeUndefined()
})

it('counts how many NEW labels the import will create, case-insensitively deduped against existing labels', () => {
  const sel = validBase()
  sel.modes.labels = 'csv_column'
  sel.columns.labels = 'Labels'
  // row 2's "kid a" differs in case from both row 1's "Kid A" and the
  // existing "Kid A" below — this is what actually exercises lowercasing on
  // both sides of the comparison, not just "some filtering happens"
  const analysis = {
    header: ['Account', 'Date', 'Amount', 'Labels'],
    rows: [
      ['Cash', '2026-01-02', '10', 'Kid A;Kid B'],
      ['Cash', '2026-01-03', '20', 'kid a'],
    ],
    samples: {},
  }
  // "kid a" case-folds against the existing "Kid A" -> only "Kid B" is new.
  // Verified by mutation: stripping both .toLowerCase() calls in
  // countNewLabels turns this 1 into 2 ("kid a" no longer matches the
  // differently-cased existing entry, so it counts as new too).
  expect(countNewLabels(analysis, sel, ['Kid A'])).toBe(1)
  // With no existing labels, "Kid A" (row 1) and "kid a" (row 2) still
  // case-fold together into one name, leaving 2 distinct new names ("kid a"
  // and "Kid B"). Verified by the same mutation: it turns this 2 into 3
  // ("Kid A" and "kid a" stop collapsing into one name).
  expect(countNewLabels(analysis, sel, [])).toBe(2)
  // an unmapped labels column contributes nothing to the preview
  const unmapped = validBase()
  expect(countNewLabels(analysis, unmapped, [])).toBe(0)
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

const accounts = fixtureAccounts as unknown as AccountDto[]
const csvSelection = (): ImportSelection => ({ ...defaultSelection(), columns: { ...defaultSelection().columns, account: 'Account' } })

it('targetAccountIds: fixed account, and csv column names matched case-insensitively', () => {
  const analysis = analyzeCsv('Account,Amount\ncash,1\nBANK,2\ncash,3\nNowhere,4\n')
  expect(targetAccountIds(analysis, csvSelection(), accounts)).toEqual(['a1', 'a2'])
  const fixed: ImportSelection = { ...defaultSelection(), modes: { ...defaultSelection().modes, account: 'existing' }, fixed: { ...defaultSelection().fixed, accountId: 'a3' } }
  expect(targetAccountIds(analysis, fixed, accounts)).toEqual(['a3'])
  expect(targetAccountIds(analysis, defaultSelection(), accounts)).toEqual([])
})

it('runImportKeepingBalance restores after a partial import with the snapshot values', async () => {
  const restores: unknown[] = []
  server.use(
    http.get('*/api/v1/account/get-balance-snapshot', ({ request }) => {
      expect(new URL(request.url).searchParams.get('accountIds')).toBe('a1')
      return HttpResponse.json({ success: true, message: '', data: { takenAt: '2026-11-07 10:00:00', items: [{ accountId: 'a1', balance: '100.5', openingCorrectionId: 'c1', fresh: true }] } })
    }),
    http.post('*/api/v1/account/restore-balance', async ({ request }) => {
      restores.push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: { item: fixtureAccounts[0], restored: true } })
    }),
  )
  const rows = Array.from({ length: 501 }, () => 'Cash,1').join('\n')
  const analysis = analyzeCsv(`Account,Amount\n${rows}\n`)
  let call = 0
  const post = vi.fn(async () => {
    call += 1
    if (call === 2) throw new Error('boom')
    return { imported: 500, skipped: 0, errors: {} }
  })
  const result = await runImportKeepingBalance(analysis, csvSelection(), post, accounts)
  expect(result.imported).toBe(500)
  expect(result.failed).toBe(1)
  expect(result.kept).toEqual([{ accountId: 'a1', balance: '100.5' }])
  expect(restores).toEqual([{ accountId: 'a1', balance: '100.5', openingCorrectionId: 'c1', since: '2026-11-07 10:00:00' }])
})

function keepHandlers(opts: { snapshot?: 'fail'; restore?: (accountId: string) => Response | Promise<Response> }, log: string[]) {
  const item = (accountId: string) => ({ accountId, balance: '10', openingCorrectionId: '', fresh: true })
  server.use(
    http.get('*/api/v1/account/get-balance-snapshot', () => {
      log.push('snapshot')
      if (opts.snapshot === 'fail') return HttpResponse.json({ success: false, message: 'x' }, { status: 500 })
      return HttpResponse.json({ success: true, message: '', data: { takenAt: '2026-11-07 10:00:00', items: [item('a1'), item('a2')] } })
    }),
    http.post('*/api/v1/account/restore-balance', async ({ request }) => {
      const { accountId } = (await request.json()) as { accountId: string }
      log.push(`restore:${accountId}`)
      return opts.restore
        ? opts.restore(accountId)
        : HttpResponse.json({ success: true, message: '', data: { item: fixtureAccounts[0], restored: true } })
    }),
  )
}
const twoAccountCsv = () => analyzeCsv('Account,Amount\nCash,1\nBank,2\n')

it('runImportKeepingBalance order: snapshot, chunk posts, then restores', async () => {
  const log: string[] = []
  keepHandlers({}, log)
  const post = vi.fn(async () => {
    log.push('post')
    return { imported: 2, skipped: 0, errors: {} }
  })
  const result = await runImportKeepingBalance(twoAccountCsv(), csvSelection(), post, accounts)
  expect(log).toEqual(['snapshot', 'post', 'restore:a1', 'restore:a2'])
  expect(result.notKept).toEqual([])
})

it('a failed snapshot still imports and marks every target notKept', async () => {
  const log: string[] = []
  keepHandlers({ snapshot: 'fail' }, log)
  const post = vi.fn(async () => ({ imported: 2, skipped: 0, errors: {} }))
  const result = await runImportKeepingBalance(twoAccountCsv(), csvSelection(), post, accounts)
  expect(post).toHaveBeenCalledTimes(1)
  expect(result.imported).toBe(2)
  expect(result.kept).toEqual([])
  expect(result.notKept).toEqual(['a1', 'a2'])
  expect(log).toEqual(['snapshot'])
})

it('a failed restore request is notKept; restored:false is neither kept nor notKept', async () => {
  keepHandlers(
    {
      restore: (id) =>
        id === 'a1'
          ? HttpResponse.json({ success: false, message: 'x' }, { status: 500 })
          : HttpResponse.json({ success: true, message: '', data: { item: fixtureAccounts[1], restored: false } }),
    },
    [],
  )
  const post = vi.fn(async () => ({ imported: 2, skipped: 0, errors: {} }))
  const result = await runImportKeepingBalance(twoAccountCsv(), csvSelection(), post, accounts)
  expect(result.kept).toEqual([])
  expect(result.notKept).toEqual(['a1'])
})

it('a target missing from the snapshot (not owned or administered) is notKept', async () => {
  const log: string[] = []
  keepHandlers({}, log)
  server.use(
    http.get('*/api/v1/account/get-balance-snapshot', () =>
      HttpResponse.json({ success: true, message: '', data: { takenAt: '2026-11-07 10:00:00', items: [{ accountId: 'a1', balance: '10', openingCorrectionId: '', fresh: true }] } }),
    ),
  )
  const post = vi.fn(async () => ({ imported: 2, skipped: 0, errors: {} }))
  const result = await runImportKeepingBalance(twoAccountCsv(), csvSelection(), post, accounts)
  expect(result.kept).toEqual([{ accountId: 'a1', balance: '10' }])
  expect(result.notKept).toEqual(['a2'])
})
