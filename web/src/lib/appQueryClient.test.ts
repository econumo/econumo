import { AxiosError, type AxiosResponse } from 'axios'
import { toast } from 'sonner'
import { queryKeys } from '@/app/queryKeys'
import { createAppQueryClient } from './appQueryClient'

vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

const noResponse = () => new AxiosError('Network Error', AxiosError.ERR_NETWORK)
const failing = (err: unknown, onError?: () => void) =>
  (() => { const client = createAppQueryClient(); return client.getMutationCache().build(client, { mutationFn: async () => { throw err }, onError }).execute(undefined) })()

beforeEach(() => vi.mocked(toast.error).mockClear())

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

it('a write that gets no response and handles no errors itself shows a toast', async () => {
  await expect(failing(noResponse())).rejects.toThrow()
  expect(toast.error).toHaveBeenCalledTimes(1)
})

it('a write with its own onError gets no second toast', async () => {
  const own = vi.fn()
  await expect(failing(noResponse(), own)).rejects.toThrow()
  expect(own).toHaveBeenCalledTimes(1)
  expect(toast.error).not.toHaveBeenCalled()
})

it('a write the server answered shows no global toast', async () => {
  const refused = new AxiosError('Bad', 'ERR_BAD_REQUEST', undefined, undefined, { status: 400, data: {} } as AxiosResponse)
  await expect(failing(refused)).rejects.toThrow()
  expect(toast.error).not.toHaveBeenCalled()
})
