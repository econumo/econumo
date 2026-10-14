import type { AccountDto } from '@/api/dto/account'
import type { AccountTransactionListDto, StoredTransaction } from '@/api/dto/transaction'
import { RELOAD_REVISION, type Bucket } from '@/lib/transactionStore'
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

  it('downloads everything on the first sync, one account per request', async () => {
    const fetch = vi.fn((ids?: string[]) => Promise.resolve(reply({ [ids![0]]: 1 })))
    const out = await syncBuckets({ accounts: [account('a', 1), account('b', 1)], stored: new Map(), full: false, fetch })
    expect(fetch.mock.calls).toEqual([[['a']], [['b']]])
    expect(out.changed.map((b) => b.accountId).sort()).toEqual(['a', 'b'])
  })

  it('downloads every stale account on its own when more than MAX_IDS are stale', async () => {
    const ids = Array.from({ length: MAX_IDS + 1 }, (_, i) => `a${i}`)
    const fetch = vi.fn((asked?: string[]) => Promise.resolve(reply({ [asked![0]]: 2 })))
    await syncBuckets({
      accounts: [account('fresh', 1), ...ids.map((id) => account(id, 2))],
      stored: stored(bucket('fresh', 1)),
      full: false,
      fetch,
    })
    expect(fetch).toHaveBeenCalledTimes(MAX_IDS + 1)
    expect(fetch.mock.calls.every(([asked]) => asked!.length === 1 && asked![0] !== 'fresh')).toBe(true)
  })

  it('downloads every account on its own when forced, even if every revision matches', async () => {
    const fetch = vi.fn((ids?: string[]) => Promise.resolve(reply({ [ids![0]]: 2 })))
    await syncBuckets({ accounts: [account('a', 2), account('b', 2)], stored: stored(bucket('a', 2), bucket('b', 2)), full: true, fetch })
    expect(fetch.mock.calls).toEqual([[['a']], [['b']]])
  })

  it('keeps at most two account downloads in flight, in the accounts order', async () => {
    const pending: { id: string; resolve: () => void }[] = []
    let inFlight = 0
    let most = 0
    const fetch = vi.fn((ids?: string[]) => {
      inFlight++
      most = Math.max(most, inFlight)
      return new Promise<AccountTransactionListDto>((resolve) => {
        pending.push({ id: ids![0], resolve: () => { inFlight--; resolve(reply({ [ids![0]]: 1 })) } })
      })
    })
    const run = syncBuckets({ accounts: [account('a', 1), account('b', 1), account('c', 1)], stored: new Map(), full: false, fetch })
    await vi.waitFor(() => expect(pending.map((p) => p.id)).toEqual(['a', 'b']))
    pending[1].resolve()
    await vi.waitFor(() => expect(pending.map((p) => p.id)).toEqual(['a', 'b', 'c']))
    pending[0].resolve()
    pending[2].resolve()
    const out = await run
    expect(most).toBe(2)
    expect([...out.buckets.keys()].sort()).toEqual(['a', 'b', 'c'])
  })

  it('hands each account to persist as it arrives, with the revision of its own response', async () => {
    const persisted: Bucket[][] = []
    const fetch = vi.fn((ids?: string[]) => Promise.resolve(reply({ [ids![0]]: ids![0] === 'a' ? 4 : 9 }, [tx(`t-${ids![0]}`, ids![0])])))
    await syncBuckets({
      accounts: [account('a', 3), account('b', 9)],
      stored: new Map(),
      full: false,
      fetch,
      persist: (buckets) => persisted.push(buckets),
    })
    expect(persisted.map((b) => b.map((x) => [x.accountId, x.revision]))).toEqual([[['a', 4]], [['b', 9]]])
  })

  it('a failed account fails the sync, after persisting the accounts that arrived', async () => {
    const persisted: string[] = []
    const fetch = vi.fn((ids?: string[]) => (ids![0] === 'c'
      ? Promise.reject(new Error('offline'))
      : Promise.resolve(reply({ [ids![0]]: 1 }))))
    await expect(syncBuckets({
      accounts: [account('a', 1), account('b', 1), account('c', 1)],
      stored: new Map(),
      full: false,
      fetch,
      persist: (buckets) => persisted.push(...buckets.map((b) => b.accountId)),
    })).rejects.toThrow('offline')
    expect(persisted.sort()).toEqual(['a', 'b'])
  })

  it('starts no further account download once cancelled', async () => {
    const controller = new AbortController()
    const fetch = vi.fn((ids?: string[]) => {
      if (ids![0] === 'b') {
        controller.abort()
      }
      return Promise.resolve(reply({ [ids![0]]: 1 }))
    })
    await expect(syncBuckets({
      accounts: ['a', 'b', 'c', 'd'].map((id) => account(id, 1)),
      stored: new Map(),
      full: false,
      fetch,
      signal: controller.signal,
    })).rejects.toThrow()
    expect(fetch.mock.calls.map(([ids]) => ids![0])).toEqual(['a', 'b'])
  })

  it('after a partial first load, downloads each missing account on its own', async () => {
    const fetch = vi.fn((ids?: string[]) => Promise.resolve(reply({ [ids![0]]: 1 })))
    const out = await syncBuckets({
      accounts: ['a', 'b', 'c', 'd'].map((id) => account(id, 1)),
      stored: stored(bucket('a', 1), bucket('b', 1)),
      full: false,
      fetch,
    })
    expect(fetch.mock.calls).toEqual([[['c']], [['d']]])
    expect([...out.buckets.keys()].sort()).toEqual(['a', 'b', 'c', 'd'])
  })

  it('batches the changed accounts and downloads the missing ones on their own', async () => {
    const persisted: string[][] = []
    const fetch = vi.fn((ids?: string[]) => Promise.resolve(reply(Object.fromEntries(ids!.map((id) => [id, 2])))))
    const out = await syncBuckets({
      accounts: ['a', 'b', 'c', 'd'].map((id) => account(id, 2)),
      stored: stored(bucket('a', 1), bucket('b', 1)),
      full: false,
      fetch,
      persist: (buckets) => persisted.push(buckets.map((x) => x.accountId)),
    })
    expect(fetch.mock.calls).toEqual([[['a', 'b']], [['c']], [['d']]])
    expect(persisted).toEqual([['a', 'b'], ['c'], ['d']])
    expect([...out.buckets.values()].every((x) => x.revision === 2)).toBe(true)
  })

  it('downloads a bucket marked for a full reload on its own, like a missing one', async () => {
    const fetch = vi.fn((ids?: string[]) => Promise.resolve(reply(Object.fromEntries(ids!.map((id) => [id, 2])))))
    await syncBuckets({
      accounts: ['a', 'b', 'c'].map((id) => account(id, 2)),
      stored: stored(bucket('a', 1), bucket('b', RELOAD_REVISION, [tx('t-b', 'b')]), bucket('c', 2)),
      full: false,
      fetch,
    })
    expect(fetch.mock.calls).toEqual([[['a']], [['b']]])
  })

  it('downloads changed accounts on their own when more than MAX_IDS of them changed', async () => {
    const ids = Array.from({ length: MAX_IDS + 1 }, (_, i) => `a${i}`)
    const fetch = vi.fn((asked?: string[]) => Promise.resolve(reply({ [asked![0]]: 2 })))
    await syncBuckets({
      accounts: ids.map((id) => account(id, 2)),
      stored: stored(...ids.map((id) => bucket(id, 1))),
      full: false,
      fetch,
    })
    expect(fetch).toHaveBeenCalledTimes(MAX_IDS + 1)
    expect(fetch.mock.calls.every(([asked]) => asked!.length === 1)).toBe(true)
  })

  it('a few stale accounts with a filled store still go in one request, persisted once', async () => {
    const persisted: Bucket[][] = []
    const fetch = vi.fn().mockResolvedValue(reply({ a: 2, b: 2 }))
    await syncBuckets({
      accounts: [account('a', 2), account('b', 2), account('c', 1)],
      stored: stored(bucket('a', 1), bucket('b', 1), bucket('c', 1)),
      full: false,
      fetch,
      persist: (buckets) => persisted.push(buckets),
    })
    expect(fetch.mock.calls).toEqual([[['a', 'b']]])
    expect(persisted.map((b) => b.map((x) => x.accountId))).toEqual([['a', 'b']])
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
    const out = await syncBuckets({ accounts: [account('a', 1), account('b', 1)], stored: stored(bucket('c', 1)), full: false, fetch })
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
    const out = await syncBuckets({ accounts: [account('a', 1), account('b', 1)], stored: stored(bucket('c', 1)), full: false, fetch })
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
