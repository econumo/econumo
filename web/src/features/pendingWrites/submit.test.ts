import { AxiosError, type AxiosResponse } from 'axios'
import { onlineManager } from '@tanstack/react-query'
import { queryKeys } from '@/app/queryKeys'
import type { AccountDto } from '@/api/dto/account'
import type { CreateTransactionDto, TransactionDto, TransactionItemDto } from '@/api/dto/transaction'
import { createAppQueryClient } from '@/lib/appQueryClient'
import { METRICS, trackEvent } from '@/lib/metrics'
import { STORE_TIMEOUT_MS } from '@/lib/transactionStore'
import { delayTransactionCompletion } from '@/test/idbDelay'
import { getOps, loadPendingWrites, mutate } from './store'
import { addChange } from './queue'
import { discardWrite, retryWrite, submitChange } from './submit'
import type { OpChange } from './types'

vi.mock('@/lib/metrics', async (original) => ({
  ...(await original<typeof import('@/lib/metrics')>()),
  trackEvent: vi.fn(),
}))

const payload = (id: string): CreateTransactionDto => ({
  id, type: 'expense', accountId: 'a1', accountRecipientId: null, amount: '5', amountRecipient: null,
  categoryId: null, description: '', payeeId: null, tagId: null, labelIds: [], date: '2026-11-08 10:00:00',
})
const create: OpChange = { kind: 'tx-create', txId: 'temp', payload: payload('temp') }
const item: TransactionItemDto = {
  item: { ...payload('real'), author: { id: 'u1', name: 'Ann', avatar: '' }, recurringId: null, isImported: 0 } as TransactionDto,
  accounts: [{ id: 'a1' } as AccountDto],
}

function httpError(status: number, data: unknown): AxiosError {
  return new AxiosError('failed', 'ERR_BAD_RESPONSE', undefined, undefined, { status, data } as AxiosResponse)
}
const envelope = { success: false, message: 'Nope', code: 0, errors: {} }

let client: ReturnType<typeof createAppQueryClient>

beforeEach(async () => {
  client = createAppQueryClient()
  client.setQueryData(queryKeys.transactions, [])
  vi.mocked(trackEvent).mockClear()
  onlineManager.setOnline(true)
  await loadPendingWrites('u1')
})

afterEach(() => {
  onlineManager.setOnline(true)
  vi.restoreAllMocks()
})

const writes = (_stores: string[], mode: IDBTransactionMode | undefined) => mode === 'readwrite'

it('has not resolved until the queued change is on disk, and shows it in memory at once', async () => {
  onlineManager.setOnline(false)
  delayTransactionCompletion(200, writes)
  let resolved = false
  const submitted = submitChange(client, create, vi.fn()).then(() => { resolved = true })
  expect(getOps()).toHaveLength(1)
  await new Promise((r) => setTimeout(r, 100))
  expect(resolved).toBe(false)
  await submitted
  expect(resolved).toBe(true)
})

it('resolves after the time limit when the disk never answers', async () => {
  onlineManager.setOnline(false)
  delayTransactionCompletion(null, writes)
  const started = Date.now()
  await expect(submitChange(client, create, vi.fn())).resolves.toEqual({ queued: true })
  expect(Date.now() - started).toBeGreaterThanOrEqual(STORE_TIMEOUT_MS - 50)
  expect(getOps()).toHaveLength(1)
})

it('offline: queues with no request and resolves at once', async () => {
  onlineManager.setOnline(false)
  const send = vi.fn()
  await expect(submitChange(client, create, send)).resolves.toEqual({ queued: true })
  expect(send).not.toHaveBeenCalled()
  expect(getOps()).toMatchObject([{ kind: 'tx-create', txId: 'temp', attempted: false, status: 'waiting', seq: 1 }])
  expect(trackEvent).toHaveBeenCalledWith(METRICS.WRITE_QUEUED, { kind: 'tx-create' })
})

it('online success returns the result, applies it, queues nothing and fires the confirm metric once', async () => {
  const send = vi.fn().mockResolvedValue(item)
  await expect(submitChange(client, create, send)).resolves.toEqual({ queued: false, result: item })
  expect(getOps()).toEqual([])
  expect(client.getQueryData<TransactionDto[]>(queryKeys.transactions)?.map((t) => t.id)).toEqual(['real'])
  expect(vi.mocked(trackEvent).mock.calls).toEqual([[METRICS.TRANSACTION_CREATE]])
})

it('passes a key and the 10 second timeout to the request', async () => {
  const send = vi.fn().mockResolvedValue(item)
  await submitChange(client, { kind: 'limit', budgetId: 'b1', elementId: 'e1', period: '2026-11-01', amount: '5' }, send)
  expect(send.mock.calls[0][0]).toEqual({ idempotencyKey: expect.stringMatching(/^[0-9a-f-]{36}$/), timeout: 10000 })
})

it('online refusal rejects and queues nothing', async () => {
  const refusal = httpError(400, envelope)
  await expect(submitChange(client, create, vi.fn().mockRejectedValue(refusal))).rejects.toBe(refusal)
  expect(getOps()).toEqual([])
})

it('a programming error rejects and queues nothing', async () => {
  const bug = new TypeError('oops')
  await expect(submitChange(client, create, vi.fn().mockRejectedValue(bug))).rejects.toBe(bug)
  expect(getOps()).toEqual([])
})

it('a timeout queues as attempted under the key the request carried', async () => {
  const send = vi.fn().mockRejectedValue(new AxiosError('timeout', AxiosError.ECONNABORTED))
  await expect(submitChange(client, create, send)).resolves.toEqual({ queued: true })
  expect(getOps()).toMatchObject([{ attempted: true, status: 'waiting', key: send.mock.calls[0][0].idempotencyKey }])
})

it('resubmitting the same add, a retap after a 5xx, carries the same key: the form id', async () => {
  const send = vi.fn().mockRejectedValueOnce(httpError(500, envelope)).mockResolvedValueOnce(item)
  await expect(submitChange(client, create, send)).rejects.toBeDefined()
  await submitChange(client, create, send)
  expect(send.mock.calls.map((c) => c[0].idempotencyKey)).toEqual(['temp', 'temp'])
})

it('an add that waits in the queue is keyed by the form id too', async () => {
  onlineManager.setOnline(false)
  await submitChange(client, create, vi.fn())
  expect(getOps()).toMatchObject([{ key: 'temp', attempted: false }])
})

it('edits and deletes get a key of their own', async () => {
  const send = vi.fn().mockResolvedValue(item)
  await submitChange(client, { kind: 'tx-update', txId: 'real', payload: payload('real') }, send)
  await submitChange(client, { kind: 'tx-update', txId: 'real', payload: payload('real') }, send)
  const [first, second] = send.mock.calls.map((c) => c[0].idempotencyKey)
  expect(first).toMatch(/^[0-9a-f-]{36}$/)
  expect(second).not.toBe(first)
})

it('a 409 on the direct path queues as attempted under the same key instead of rejecting', async () => {
  const send = vi.fn().mockRejectedValue(httpError(409, { ...envelope, code: 409, resourceId: 'real' }))
  await expect(submitChange(client, create, send)).resolves.toEqual({ queued: true })
  expect(getOps()).toMatchObject([{ kind: 'tx-create', key: 'temp', attempted: true, status: 'waiting' }])
})

it('the same add again while the first is in the queue goes behind it as an edit, under a key of its own', async () => {
  await mutate((ops) => addChange(ops, create, { key: 'temp', seq: 1 }, true))
  const send = vi.fn()
  await submitChange(client, { ...create, payload: { ...payload('temp'), amount: '9' } } as OpChange, send)
  expect(send).not.toHaveBeenCalled()
  expect(getOps()).toHaveLength(2)
  expect(getOps()[0]).toMatchObject({ kind: 'tx-create', key: 'temp' })
  expect(getOps()[1]).toMatchObject({ kind: 'tx-update', txId: 'temp', payload: { amount: '9' } })
  expect(getOps()[1].key).not.toBe('temp')
})

it('a timed-out change takes its seq from the queue as it stands then, and goes ahead of a later edit', async () => {
  let fail: (e: unknown) => void = () => {}
  const send = vi.fn(() => new Promise<never>((_, reject) => { fail = reject }))
  const submitted = submitChange(client, create, send)
  // the edit made while the request hangs
  await mutate((ops) => addChange(ops, { kind: 'tx-update', txId: 'temp', payload: payload('temp') }, { key: 'later', seq: 7 }))

  fail(new AxiosError('timeout', AxiosError.ECONNABORTED))
  await submitted
  const ordered = [...getOps()].sort((a, b) => a.seq - b.seq)
  expect(ordered.map((o) => o.kind)).toEqual(['tx-create', 'tx-update'])
  expect(ordered.map((o) => o.seq)).toEqual([7, 8])
})

it('a waiting change on the target queues without a request, folding into it', async () => {
  onlineManager.setOnline(false)
  await submitChange(client, create, vi.fn())
  onlineManager.setOnline(true)
  const send = vi.fn()
  const edit = { ...payload('temp'), amount: '9' }
  await expect(submitChange(client, { kind: 'tx-update', txId: 'temp', payload: edit }, send)).resolves.toEqual({ queued: true })
  expect(send).not.toHaveBeenCalled()
  expect(getOps()).toHaveLength(1)
  expect(getOps()[0]).toMatchObject({ kind: 'tx-create', payload: { amount: '9' } })
})

it('a failed change on the target queues without a request and becomes waiting', async () => {
  await mutate((ops) => addChange(ops, create, { key: 'k', seq: 1 }).map((o) => ({ ...o, status: 'failed' as const, error: 'No' })))
  const send = vi.fn()
  await expect(submitChange(client, { kind: 'tx-update', txId: 'temp', payload: payload('temp') }, send)).resolves.toEqual({ queued: true })
  expect(send).not.toHaveBeenCalled()
  expect(getOps()).toMatchObject([{ key: 'k', status: 'waiting' }])
})

it('retryWrite makes a failed change waiting and fires its metric', async () => {
  await mutate((ops) => addChange(ops, create, { key: 'k', seq: 1 }).map((o) => ({ ...o, status: 'failed' as const, error: 'No' })))
  retryWrite('k')
  expect(getOps()).toMatchObject([{ status: 'waiting' }])
  expect(trackEvent).toHaveBeenCalledWith(METRICS.WRITE_RETRIED, { kind: 'tx-create' })
})

it('discardWrite drops the change and fires its metric', async () => {
  await mutate((ops) => addChange(ops, create, { key: 'k', seq: 1 }))
  discardWrite('k')
  expect(getOps()).toEqual([])
  expect(trackEvent).toHaveBeenCalledWith(METRICS.WRITE_DISCARDED, { kind: 'tx-create' })
})
