import type { AccountDto } from '@/api/dto/account'
import type { AccountTransactionListDto, StoredTransaction } from '@/api/dto/transaction'
import type { Bucket } from '@/lib/transactionStore'
import { MAX_IDS, assemble, hasRevisions, syncBuckets } from './sync'

const ada = { id: 'u1', name: 'Ada', avatar: 'face:emerald' }

function account(id: string, transactionsRevision?: number): AccountDto {
  return { id, transactionsRevision } as AccountDto
}

function tx(id: string, accountId: string, accountRecipientId: string | null = null): StoredTransaction {
  return { id, accountId, accountRecipientId, authorId: 'u1' } as StoredTransaction
}

function bucket(accountId: string, revision: number, items: StoredTransaction[] = []): Bucket {
  return { accountId, revision, items, authors: items.length ? [ada] : [] }
}

function stored(...buckets: Bucket[]): Map<string, Bucket> {
  return new Map(buckets.map((b) => [b.accountId, b]))
}

function reply(revisions: Record<string, number>, items: StoredTransaction[] = []): AccountTransactionListDto {
  return { revisions, items, authors: items.length ? [ada] : [] }
}

describe('syncBuckets', () => {
  it('fetches an account that has no stored bucket', async () => {
    const fetch = vi.fn().mockResolvedValue(reply({ a: 3 }, [tx('t1', 'a')]))
    const out = await syncBuckets({
      accounts: [account('a', 3), account('b', 1)],
      stored: stored(bucket('b', 1)),
      full: false,
      fetch,
    })
    expect(fetch).toHaveBeenCalledWith(['a'])
    expect(out.changed.map((b) => b.accountId)).toEqual(['a'])
    expect(out.buckets.get('a')?.revision).toBe(3)
    expect(out.buckets.get('a')?.items.map((t) => t.id)).toEqual(['t1'])
    expect(out.buckets.get('b')?.revision).toBe(1)
    expect(out.removed).toEqual([])
  })

  it('does not fetch when every revision matches', async () => {
    const fetch = vi.fn()
    const input = stored(bucket('a', 2), bucket('b', 5))
    const out = await syncBuckets({ accounts: [account('a', 2), account('b', 5)], stored: input, full: false, fetch })
    expect(fetch).not.toHaveBeenCalled()
    expect(out.buckets).toEqual(input)
    expect(out.changed).toEqual([])
    expect(out.removed).toEqual([])
  })

  it('fetches only the changed account of three', async () => {
    const fetch = vi.fn().mockResolvedValue(reply({ b: 9 }, [tx('t9', 'b')]))
    const out = await syncBuckets({
      accounts: [account('a', 1), account('b', 9), account('c', 1)],
      stored: stored(bucket('a', 1), bucket('b', 4, [tx('old', 'b')]), bucket('c', 1)),
      full: false,
      fetch,
    })
    expect(fetch).toHaveBeenCalledWith(['b'])
    expect(out.changed.map((b) => b.accountId)).toEqual(['b'])
    expect(out.buckets.get('b')?.items.map((t) => t.id)).toEqual(['t9'])
  })

  it('downloads everything on the first sync', async () => {
    const fetch = vi.fn().mockResolvedValue(reply({ a: 1, b: 1 }))
    const out = await syncBuckets({ accounts: [account('a', 1), account('b', 1)], stored: new Map(), full: false, fetch })
    expect(fetch).toHaveBeenCalledWith(undefined)
    expect(out.changed.map((b) => b.accountId).sort()).toEqual(['a', 'b'])
  })

  it('downloads everything when more than MAX_IDS accounts are stale', async () => {
    const ids = Array.from({ length: MAX_IDS + 1 }, (_, i) => `a${i}`)
    const revisions = Object.fromEntries(ids.map((id) => [id, 2]))
    const fetch = vi.fn().mockResolvedValue(reply(revisions))
    await syncBuckets({
      accounts: ids.map((id) => account(id, 2)),
      stored: stored(bucket('a0', 1)),
      full: false,
      fetch,
    })
    expect(fetch).toHaveBeenCalledWith(undefined)
  })

  it('downloads everything when forced, even if every revision matches', async () => {
    const fetch = vi.fn().mockResolvedValue(reply({ a: 2 }))
    await syncBuckets({ accounts: [account('a', 2)], stored: stored(bucket('a', 2)), full: true, fetch })
    expect(fetch).toHaveBeenCalledWith(undefined)
  })

  it('removes buckets of accounts that left the list', async () => {
    const fetch = vi.fn()
    const out = await syncBuckets({
      accounts: [account('a', 1)],
      stored: stored(bucket('a', 1), bucket('gone', 1)),
      full: false,
      fetch,
    })
    expect(fetch).not.toHaveBeenCalled()
    expect(out.removed).toEqual(['gone'])
    expect([...out.buckets.keys()]).toEqual(['a'])
  })

  it('removes requested accounts the server no longer reports', async () => {
    const fetch = vi.fn().mockResolvedValue(reply({ a: 2 }))
    const out = await syncBuckets({
      accounts: [account('a', 2), account('b', 7)],
      stored: stored(bucket('a', 1), bucket('b', 1)),
      full: false,
      fetch,
    })
    expect(out.removed).toEqual(['b'])
    expect(out.buckets.has('b')).toBe(false)
    expect(out.buckets.get('a')?.revision).toBe(2)
  })

  it('files a transfer under both accounts and assembles it once', async () => {
    const transfer = tx('tr', 'a', 'b')
    const fetch = vi.fn().mockResolvedValue(reply({ a: 1, b: 1 }, [transfer, tx('only-b', 'b')]))
    const out = await syncBuckets({ accounts: [account('a', 1), account('b', 1)], stored: new Map(), full: false, fetch })
    expect(out.buckets.get('a')?.items.map((t) => t.id)).toEqual(['tr'])
    expect(out.buckets.get('b')?.items.map((t) => t.id)).toEqual(['tr', 'only-b'])
    expect(assemble(out.buckets).map((t) => t.id).sort()).toEqual(['only-b', 'tr'])
  })

  it('files a row only under the requested accounts it touches, once each', async () => {
    const fetch = vi.fn().mockResolvedValue(reply({ a: 2 }, [tx('self', 'a', 'a'), tx('out', 'a', 'x'), tx('in', 'y', 'a')]))
    const out = await syncBuckets({
      accounts: [account('a', 2), account('b', 1)],
      stored: stored(bucket('b', 1, [tx('kept', 'b')])),
      full: false,
      fetch,
    })
    expect(out.buckets.get('a')?.items.map((t) => t.id)).toEqual(['self', 'out', 'in'])
    expect(out.buckets.get('b')?.items.map((t) => t.id)).toEqual(['kept'])
    expect(out.buckets.has('x')).toBe(false)
    expect(out.buckets.has('y')).toBe(false)
  })

  it('keeps only the authors a bucket references', async () => {
    const bob = { id: 'u2', name: 'Bob', avatar: '' }
    const fetch = vi.fn().mockResolvedValue({
      revisions: { a: 1, b: 1 },
      items: [tx('t1', 'a')],
      authors: [ada, bob],
    })
    const out = await syncBuckets({ accounts: [account('a', 1), account('b', 1)], stored: new Map(), full: false, fetch })
    expect(out.buckets.get('a')?.authors).toEqual([ada])
    expect(out.buckets.get('b')?.authors).toEqual([])
  })
})

describe('assemble', () => {
  it('fills the author from the bucket authors', () => {
    const out = assemble(stored(bucket('a', 1, [tx('t1', 'a')])))
    expect(out[0].author).toEqual(ada)
    expect(out[0]).not.toHaveProperty('authorId')
  })

  it('falls back to an empty author when it is unknown', () => {
    const b: Bucket = { accountId: 'a', revision: 1, items: [tx('t1', 'a')], authors: [] }
    expect(assemble(stored(b))[0].author).toEqual({ id: 'u1', name: '', avatar: '' })
  })
})

describe('hasRevisions', () => {
  it('is true only when every account carries a numeric revision', () => {
    expect(hasRevisions([account('a', 0), account('b', 4)])).toBe(true)
    expect(hasRevisions([account('a', 1), account('b')])).toBe(false)
  })
})
