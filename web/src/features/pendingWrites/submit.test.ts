import { AxiosError, type AxiosResponse } from 'axios'
import { onlineManager } from '@tanstack/react-query'
import { queryKeys } from '@/app/queryKeys'
import type { AccountDto } from '@/api/dto/account'
import type { CreateTransactionDto, TransactionDto, TransactionItemDto } from '@/api/dto/transaction'
import { createAppQueryClient } from '@/lib/appQueryClient'
import { METRICS, trackEvent } from '@/lib/metrics'
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

afterEach(() => onlineManager.setOnline(true))

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
  await submitChange(client, create, send)
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
