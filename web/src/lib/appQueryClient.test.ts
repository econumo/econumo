import { onlineManager } from '@tanstack/react-query'
import { queryKeys } from '@/app/queryKeys'
import { createAppQueryClient } from './appQueryClient'

it('any successful write marks the reports stale, and only the reports', async () => {
  const client = createAppQueryClient()
  client.setQueryData([...queryKeys.reportNetWorth, 'mine'], { months: [] })
  client.setQueryData(queryKeys.categories, [])
  await client.getMutationCache().build(client, { mutationFn: async () => 'ok' }).execute(undefined)
  expect(client.getQueryState([...queryKeys.reportNetWorth, 'mine'])?.isInvalidated).toBe(true)
  expect(client.getQueryState(queryKeys.categories)?.isInvalidated).toBe(false)
})

it('a failed write leaves the reports alone', async () => {
  const client = createAppQueryClient()
  client.setQueryData([...queryKeys.reportNetWorth, 'mine'], { months: [] })
  await expect(
    client.getMutationCache().build(client, { mutationFn: async () => { throw new Error('no') }, retry: false }).execute(undefined),
  ).rejects.toThrow('no')
  expect(client.getQueryState([...queryKeys.reportNetWorth, 'mine'])?.isInvalidated).toBe(false)
})

describe('started with no connection', () => {
  beforeEach(() => {
    vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(false)
    onTestFinished(() => {
      vi.restoreAllMocks()
      onlineManager.setOnline(true)
    })
  })

  it('a read waits for the connection instead of retrying', async () => {
    const client = createAppQueryClient()
    const queryFn = vi.fn(async () => 'ok')
    void client.fetchQuery({ queryKey: ['offline-read'], queryFn })
    await vi.waitFor(() => expect(client.getQueryState(['offline-read'])?.fetchStatus).toBe('paused'))
    expect(queryFn).not.toHaveBeenCalled()
    expect(client.isFetching()).toBe(0)
  })

  it('a write still fails at once rather than hanging', async () => {
    const client = createAppQueryClient()
    await expect(
      client.getMutationCache().build(client, { mutationFn: async () => { throw new Error('no network') } }).execute(undefined),
    ).rejects.toThrow('no network')
  })
})
