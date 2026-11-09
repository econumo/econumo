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

it('a mutation fails at once when offline instead of pausing, with no retry', () => {
  const { mutations } = createAppQueryClient().getDefaultOptions()
  expect(mutations).toMatchObject({ networkMode: 'always', retry: false })
})
