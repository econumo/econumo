import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAccount } from '@/api/account'
import { ensureAccounts, runChunks, type ImportPoster } from './upload'
import type { WizardAnswers } from './prepare'

vi.mock('@/api/account', () => ({ createAccount: vi.fn(async () => ({})) }))

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

  it('maps server rows (header = 1) through fileRows, 0 stays row-less', async () => {
    const upload = makeUpload(600)
    upload.fileRows = upload.fileRows.map((r) => r * 10)
    const post = vi.fn<ImportPoster>(async (file) =>
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
    const skipped = [
      { fileRow: 3, reason: 'no_date' },
      { fileRow: 5, reason: 'no_date' },
      { fileRow: 6, reason: 'no_amount' },
    ] as never
    const result = await runChunks(makeUpload(1), post, { skipped })
    expect(result.failed).toBe(3)
    expect(result.errors).toEqual([
      { message: 'transactions.import_wizard.skipped.no_date', rows: [3, 5] },
      { message: 'transactions.import_wizard.skipped.no_amount', rows: [6] },
    ])
  })
})

describe('ensureAccounts', () => {
  beforeEach(() => vi.mocked(createAccount).mockClear())

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

  it('creates column accounts missing from existing names and created', async () => {
    const created = new Map<string, string>([['b', 'id-b']])
    const a = answers({ kind: 'column', column: 0, currencies: { a: 'c1', b: 'c1', c: 'c2' } })
    await ensureAccounts(a, created, new Set(['c']))
    expect(createAccount).toHaveBeenCalledTimes(1)
    expect(vi.mocked(createAccount).mock.calls[0][0]).toMatchObject({ name: 'a', currencyId: 'c1' })
  })
})
