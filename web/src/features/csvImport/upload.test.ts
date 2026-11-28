import { AxiosError, type AxiosResponse } from 'axios'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAccount, getFolderList } from '@/api/account'
import { ensureAccounts, mergeRetry, runChunks, type ImportPoster } from './upload'
import type { ImportResultDto } from '@/api/transaction'
import type { PreparedRow, WizardAnswers } from './prepare'

vi.mock('@/api/account', () => ({
  createAccount: vi.fn(async () => ({ item: { id: 'server-id', folderId: 'general' }, transaction: null })),
  getFolderList: vi.fn(async () => [{ id: 'made-by-server', name: 'General', position: 0, isVisible: 1 }]),
}))

// what the server answers a key it has already seen
function replay(resourceId?: string) {
  return new AxiosError('conflict', 'ERR_BAD_REQUEST', undefined, undefined, {
    status: 409,
    data: { success: false, message: 'Operation is locked', code: 409, errors: {}, ...(resourceId ? { resourceId } : {}) },
  } as AxiosResponse)
}

function makeUpload(count: number) {
  return {
    header: ['date', 'amount'],
    rows: Array.from({ length: count }, (_, i) => ['2024-01-01', String(i)]),
    fileRows: Array.from({ length: count }, (_, i) => i + 2),
    mapping: { date: 'date', amount: 'amount' },
    fields: {},
  }
}

describe('runChunks', () => {
  it('posts 1200 rows in 3 chunks', async () => {
    const post = vi.fn<ImportPoster>(async () => ({ imported: 1, skipped: 0, errors: {} }))
    const progress = vi.fn()
    const result = await runChunks(makeUpload(1200), post, { onProgress: progress })
    expect(post).toHaveBeenCalledTimes(3)
    expect(result.imported).toBe(3)
    expect(result.failedChunks).toEqual([])
    expect(progress).toHaveBeenLastCalledWith(3, 3)
  })

  it('records a thrown chunk and keeps going', async () => {
    let n = 0
    const post = vi.fn<ImportPoster>(async () => {
      if (++n === 2) throw new Error('boom')
      return { imported: 500, skipped: 0, errors: {} }
    })
    const result = await runChunks(makeUpload(1200), post)
    expect(result.failedChunks).toEqual([1])
    expect(result.imported).toBe(1000)
    expect(result.failed).toBe(500)
    expect(result.errors).toHaveLength(1)
    expect(result.errors[0].rows[0]).toBe(502)
    expect(result.errors[0].rows).toHaveLength(500)
    expect(result.errors[0].rows.at(-1)).toBe(1001)
  })

  it('sends only the requested chunks', async () => {
    const post = vi.fn<ImportPoster>(async () => ({ imported: 5, skipped: 0, errors: {} }))
    const result = await runChunks(makeUpload(1200), post, { chunks: [1] })
    expect(post).toHaveBeenCalledTimes(1)
    expect(result.imported).toBe(5)
  })

  it('counts every row of a chunk the server refused as a whole as failed, not to be sent again', async () => {
    const upload = makeUpload(3)
    const post = vi.fn<ImportPoster>(async () => ({ imported: 0, skipped: 0, errors: { 'Account not found for provided accountId': [] } }))
    const result = await runChunks(upload, post)
    expect(result).toEqual({
      imported: 0,
      failed: 3,
      errors: [{ message: 'Account not found for provided accountId', rows: [2, 3, 4] }],
      failedChunks: [],
    })
  })

  it('maps server rows (header = 1) through fileRows, 0 stays row-less', async () => {
    const upload = makeUpload(600)
    upload.fileRows = upload.fileRows.map((r) => r * 10)
    const post = vi.fn<ImportPoster>(async (file): Promise<ImportResultDto> =>
      file.name === 'chunk_0.csv'
        ? { imported: 0, skipped: 2, errors: { bad: [2, 3], top: [0] } }
        : { imported: 0, skipped: 1, errors: { bad: [2] } },
    )
    const result = await runChunks(upload, post)
    expect(result.errors).toEqual([
      { message: 'bad', rows: [20, 30, 5020] },
      { message: 'top', rows: [] },
    ])
    expect(result.failed).toBe(3)
  })

  it('counts browser-skipped rows by reason', async () => {
    const post = vi.fn<ImportPoster>(async () => ({ imported: 1, skipped: 0, errors: {} }))
    const skippedRow = (fileRow: number, reason: PreparedRow['reason']): PreparedRow => ({
      fileRow,
      raw: '',
      status: 'skipped',
      reason,
      date: null,
      time: null,
      amount: null,
      account: null,
      description: '',
      payee: '',
      category: '',
      tag: '',
      labels: '',
    })
    const skipped = [skippedRow(3, 'no_date'), skippedRow(5, 'no_date'), skippedRow(6, 'no_amount')]
    const result = await runChunks(makeUpload(1), post, { skipped })
    expect(result.failed).toBe(3)
    expect(result.errors).toEqual([
      { message: 'transactions.import_wizard.skipped.no_date', rows: [3, 5] },
      { message: 'transactions.import_wizard.skipped.no_amount', rows: [6] },
    ])
  })

  it('resends a chunk with the key it was first sent with, a different key per chunk', async () => {
    const upload = makeUpload(1200)
    const post = vi.fn<ImportPoster>(async (file) => {
      if (file.name === 'chunk_1.csv') throw new Error('timeout')
      return { imported: 500, skipped: 0, errors: {} }
    })
    await runChunks(upload, post)
    await runChunks(upload, post, { chunks: [1] })
    const keys = post.mock.calls.map((call) => call[3]?.idempotencyKey)
    expect(new Set(keys.slice(0, 3)).size).toBe(3)
    expect(keys[3]).toBe(keys[1])
    await runChunks(makeUpload(1200), post, { chunks: [1] })
    expect(post.mock.calls[4][3]?.idempotencyKey).not.toBe(keys[1])
  })

  // the replay says the request succeeded, not how many of its rows landed (a row-level skip,
  // or a refusal of the whole chunk, answers 200 too)
  it('counts a chunk the server already took as unconfirmed, neither imported nor failed', async () => {
    const post = vi.fn<ImportPoster>(async () => {
      throw replay()
    })
    const upload = makeUpload(502)
    const result = await runChunks(upload, post, { chunks: [1] })
    expect(result).toEqual({
      imported: 0,
      failed: 0,
      unconfirmed: 2,
      errors: [{ message: 'transactions.import_wizard.skipped.unconfirmed', rows: [502, 503] }],
      failedChunks: [],
    })
  })

  it('a retry answered by a replay turns its failed rows into unconfirmed ones', () => {
    const upload = makeUpload(3)
    const first = { imported: 0, failed: 3, errors: [{ message: 'Chunk 1 failed: x', rows: [2, 3, 4] }], failedChunks: [0] }
    const again = { imported: 0, failed: 0, unconfirmed: 3, errors: [{ message: 'transactions.import_wizard.skipped.unconfirmed', rows: [2, 3, 4] }], failedChunks: [] }
    expect(mergeRetry(first, again, upload, [0])).toMatchObject({ imported: 0, failed: 0, unconfirmed: 3, failedChunks: [] })
  })
})

describe('ensureAccounts', () => {
  beforeEach(() => {
    vi.mocked(createAccount).mockClear()
    vi.mocked(getFolderList).mockClear()
  })

  const answers = (account: WizardAnswers['account']) => ({ account }) as WizardAnswers

  it('creates a new account once across calls', async () => {
    const created = new Map<string, string>()
    const a = answers({ kind: 'new', name: 'Cash', currencyId: 'c1' })
    const first = await ensureAccounts(a, created, new Set())
    const second = await ensureAccounts(a, created, new Set())
    expect(createAccount).toHaveBeenCalledTimes(1)
    expect(second.accountId).toBe(first.accountId)
    expect(vi.mocked(createAccount).mock.calls[0][0]).toMatchObject({ name: 'Cash', balance: '0', folderId: null })
  })

  // the request id only guards the operation: the server mints the account's own id
  it('imports into the id the server gave the new account', async () => {
    const result = await ensureAccounts(answers({ kind: 'new', name: 'Cash', currencyId: 'c1' }), new Map(), new Set())
    expect(result.accountId).toBe('server-id')
    expect(vi.mocked(createAccount).mock.calls[0][0].id).not.toBe('server-id')
  })

  it('creates column accounts missing from existing names and created', async () => {
    const created = new Map<string, string>()
    await ensureAccounts(answers({ kind: 'column', column: 0, currencyColumn: null, currencyId: '', overrides: {}, currencies: { b: 'c1' } }), created, new Set())
    vi.mocked(createAccount).mockClear()
    const a = answers({ kind: 'column', column: 0, currencyColumn: null, currencyId: '', overrides: {}, currencies: { a: 'c1', b: 'c1', c: 'c2' } })
    await ensureAccounts(a, created, new Set(['c']))
    expect(createAccount).toHaveBeenCalledTimes(1)
    expect(vi.mocked(createAccount).mock.calls[0][0]).toMatchObject({ name: 'a', currencyId: 'c1' })
  })

  it('retries a failed create with the same request id and key', async () => {
    vi.mocked(createAccount).mockRejectedValueOnce(new Error('timeout'))
    const created = new Map<string, string>()
    const a = answers({ kind: 'new', name: 'Cash', currencyId: 'c1' })
    await expect(ensureAccounts(a, created, new Set())).rejects.toThrow('timeout')
    expect(await ensureAccounts(a, created, new Set())).toEqual({ accountId: 'server-id' })
    const [first, second] = vi.mocked(createAccount).mock.calls
    expect(second[0].id).toBe(first[0].id)
    expect(second[1]?.idempotencyKey).toBe(first[1]?.idempotencyKey)
    expect(first[1]?.idempotencyKey).toMatch(/^[0-9a-f-]{36}$/)
  })

  it('an edited name or currency after a lost create is a new account with a request of its own', async () => {
    vi.mocked(createAccount).mockRejectedValueOnce(new Error('timeout'))
    const created = new Map<string, string>()
    await expect(ensureAccounts(answers({ kind: 'new', name: 'Cash', currencyId: 'c1' }), created, new Set())).rejects.toThrow('timeout')
    await ensureAccounts(answers({ kind: 'new', name: 'Wallet', currencyId: 'c1' }), created, new Set())
    await ensureAccounts(answers({ kind: 'new', name: 'Wallet', currencyId: 'c2' }), created, new Set())
    await ensureAccounts(answers({ kind: 'new', name: 'Wallet', currencyId: 'c2' }), created, new Set())
    const calls = vi.mocked(createAccount).mock.calls
    expect(calls.map((call) => [call[0].name, call[0].currencyId])).toEqual([['Cash', 'c1'], ['Wallet', 'c1'], ['Wallet', 'c2']])
    expect(new Set(calls.map((call) => call[1]?.idempotencyKey)).size).toBe(3)
    expect(new Set(calls.map((call) => call[0].id)).size).toBe(3)
  })

  it('imports into the account a lost create made, as the replay names it', async () => {
    vi.mocked(createAccount).mockRejectedValueOnce(replay('landed-id'))
    const result = await ensureAccounts(answers({ kind: 'new', name: 'Cash', currencyId: 'c1' }), new Map(), new Set())
    expect(result.accountId).toBe('landed-id')
  })

  it('reports a replay that names no account as a failure', async () => {
    vi.mocked(createAccount).mockRejectedValueOnce(replay())
    await expect(ensureAccounts(answers({ kind: 'new', name: 'Cash', currencyId: 'c1' }), new Map(), new Set())).rejects.toBeInstanceOf(AxiosError)
  })

  it('never takes a per-row account named "new" for the new account', async () => {
    const created = new Map<string, string>()
    vi.mocked(createAccount).mockResolvedValueOnce({ item: { id: 'row-new', folderId: 'general' }, transaction: null } as never)
    await ensureAccounts(answers({ kind: 'column', column: 0, currencyColumn: null, currencyId: '', overrides: {}, currencies: { new: 'c1' } }), created, new Set())
    const result = await ensureAccounts(answers({ kind: 'new', name: 'Cash', currencyId: 'c1' }), created, new Set())
    expect(result.accountId).toBe('server-id')
    expect(createAccount).toHaveBeenCalledTimes(2)
  })

  // the server only invents a folder for a user who has none and refuses a blank one otherwise
  it('puts a new account in the folder it is given', async () => {
    await ensureAccounts(answers({ kind: 'new', name: 'Cash', currencyId: 'c1' }), new Map(), new Set(), undefined, 'f1')
    expect(vi.mocked(createAccount).mock.calls[0][0]).toMatchObject({ folderId: 'f1' })
  })

  it('puts later accounts in the folder the first one landed in', async () => {
    const a = answers({ kind: 'column', column: 0, currencyColumn: null, currencyId: '', overrides: {}, currencies: { a: 'c1', b: 'c1' } })
    await ensureAccounts(a, new Map(), new Set(), undefined, null)
    expect(vi.mocked(createAccount).mock.calls.map((call) => call[0].folderId)).toEqual([null, 'general'])
  })

  it('reads back the folder the server made when the first create was replayed', async () => {
    vi.mocked(createAccount).mockRejectedValueOnce(replay('first-id'))
    const a = answers({ kind: 'column', column: 0, currencyColumn: null, currencyId: '', overrides: {}, currencies: { a: 'c1', b: 'c1' } })
    await ensureAccounts(a, new Map(), new Set(), undefined, null)
    expect(vi.mocked(createAccount).mock.calls.map((call) => call[0].folderId)).toEqual([null, 'made-by-server'])
  })

  it('reads back the folder when an earlier attempt made an account and the folder list is stale', async () => {
    const created = new Map<string, string>()
    vi.mocked(createAccount).mockResolvedValueOnce({ item: { id: 'a-id', folderId: 'general' }, transaction: null } as never)
    vi.mocked(createAccount).mockRejectedValueOnce(new Error('timeout'))
    const a = answers({ kind: 'column', column: 0, currencyColumn: null, currencyId: '', overrides: {}, currencies: { a: 'c1', b: 'c1' } })
    await expect(ensureAccounts(a, created, new Set(), undefined, null)).rejects.toThrow('timeout')
    await ensureAccounts(a, created, new Set(), undefined, null)
    expect(vi.mocked(createAccount).mock.calls.map((call) => call[0].folderId)).toEqual([null, 'general', 'made-by-server'])
  })

  it('makes no account for a name that only left-out rows carry', async () => {
    const a = answers({ kind: 'column', column: 0, currencyColumn: null, currencyId: '', overrides: {}, currencies: { kept: 'c1', 'left out': 'c1' } })
    await ensureAccounts(a, new Map(), new Set(), new Map([['kept', 'Kept']]))
    expect(vi.mocked(createAccount).mock.calls.map((call) => call[0].name)).toEqual(['Kept'])
  })

  it('names a column account as the file writes it', async () => {
    const a = answers({ kind: 'column', column: 0, currencyColumn: null, currencyId: '', overrides: {}, currencies: { 'my card': 'c1' } })
    await ensureAccounts(a, new Map(), new Set(), new Map([['my card', 'My Card']]))
    expect(vi.mocked(createAccount).mock.calls[0][0]).toMatchObject({ name: 'My Card' })
  })
  it('creates a per-row account in the default currency unless it has its own', async () => {
    const a = answers({ kind: 'column', column: 0, currencyColumn: null, currencyId: 'cad', overrides: { b: 'usd' }, currencies: { b: 'usd' } })
    await ensureAccounts(a, new Map(), new Set(), new Map([['a', 'A'], ['b', 'B']]))
    expect(vi.mocked(createAccount).mock.calls.map((call) => [call[0].name, call[0].currencyId])).toEqual([['A', 'cad'], ['B', 'usd']])
  })
})

describe('mergeRetry', () => {
  it('counts the retried chunk by the retry and keeps the rest of the first run', async () => {
    const upload = makeUpload(1200)
    const first = await runChunks(upload, async (file) => {
      if (file.name === 'chunk_1.csv') throw new Error('down')
      return { imported: 1, skipped: 1, errors: { 'Invalid amount': [2] } }
    })
    expect(first.failedChunks).toEqual([1])
    const again = await runChunks(upload, async () => ({ imported: 499, skipped: 1, errors: { 'Invalid amount': [3] } }), { chunks: [1] })
    const merged = mergeRetry(first, again, upload, [1])
    expect(merged.imported).toBe(2 + 499)
    expect(merged.failed).toBe(2 + 1)
    expect(merged.failedChunks).toEqual([])
    expect(merged.errors).toEqual([{ message: 'Invalid amount', rows: [2, 503, 1002] }])
  })

})
