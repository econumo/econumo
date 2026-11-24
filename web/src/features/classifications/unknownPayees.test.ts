import { QueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/app/queryKeys'
import { refetchPayeesIfUnknown, resetUnknownPayees } from './unknownPayees'

const payee = (id: string) => ({ id, ownerUserId: 'u1', name: id, position: 0, isArchived: 0, createdAt: '', updatedAt: '' })

beforeEach(() => resetUnknownPayees())

it('refetches the payee list for an id it does not hold, once per id', () => {
  const queryClient = new QueryClient()
  queryClient.setQueryData(queryKeys.payees, [payee('p1')])
  const spy = vi.spyOn(queryClient, 'invalidateQueries')
  refetchPayeesIfUnknown(queryClient, ['p1', null, undefined])
  expect(spy).not.toHaveBeenCalled()
  refetchPayeesIfUnknown(queryClient, ['p1', 'p2'])
  expect(spy.mock.calls).toEqual([[{ queryKey: queryKeys.payees }]])
  // the refetch did not bring it (deleted, or not in this user's list): no second refetch
  refetchPayeesIfUnknown(queryClient, ['p2'])
  expect(spy).toHaveBeenCalledTimes(1)
  refetchPayeesIfUnknown(queryClient, ['p2', 'p3'])
  expect(spy).toHaveBeenCalledTimes(2)
})

it('does nothing while the payee list was never loaded: its first read fetches it anyway', () => {
  const queryClient = new QueryClient()
  const spy = vi.spyOn(queryClient, 'invalidateQueries')
  refetchPayeesIfUnknown(queryClient, ['p2'])
  expect(spy).not.toHaveBeenCalled()
})
