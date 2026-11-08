import { waitFor } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { fixtureOwner, fixtureUser } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { hasRealTransactions, markGettingStartedDone, storedStepState, withStepState } from './gettingStarted'

const userWith = (value: string | null) => ({
  ...fixtureUser,
  options: value === null ? fixtureUser.options : [...fixtureUser.options, { name: 'getting_started', value }],
})

const correction = (id: string, accountId: string) => ({
  id, author: fixtureOwner, type: 'income', accountId, accountRecipientId: null, amount: '100', amountRecipient: '100',
  categoryId: null, description: '', payeeId: null, tagId: null, labelIds: [], recurringId: null, date: '2026-07-01 00:00:00',
})

it('opening corrections are not real data; a categorized transaction is', () => {
  const real = { ...correction('t9', 'a1'), categoryId: 'cat-salary' }
  expect(hasRealTransactions([correction('c1', 'a1')] as never)).toBe(false)
  expect(hasRealTransactions([correction('c1', 'a1'), correction('c2', 'a2')] as never)).toBe(false)
  expect(hasRealTransactions([correction('c1', 'a1'), real] as never)).toBe(true)
})

it('a balance edit is not real data: its adjustment carries a description and may be the second on the account', () => {
  const adjustment = { ...correction('c2', 'a1'), type: 'expense', description: 'Balance adjustment' }
  expect(hasRealTransactions([correction('c1', 'a1'), adjustment] as never)).toBe(false)
  for (const field of ['payeeId', 'tagId', 'recurringId', 'accountRecipientId']) {
    expect(hasRealTransactions([{ ...adjustment, [field]: 'x' }] as never)).toBe(true)
  }
  expect(hasRealTransactions([{ ...adjustment, labelIds: ['l1'] }] as never)).toBe(true)
  expect(hasRealTransactions([{ ...adjustment, type: 'transfer' }] as never)).toBe(true)
})

it('a bare row the user described is real; only empty or server-written correction descriptions are not', () => {
  expect(hasRealTransactions([correction('c1', 'a1')] as never)).toBe(false)
  expect(hasRealTransactions([{ ...correction('t1', 'a1'), type: 'expense', description: 'Coffee' }] as never)).toBe(true)
  for (const description of ['Balance adjustment (account deleted)', 'Balance adjustment (deleted transaction)', 'Balance adjustment (edited transaction)']) {
    expect(hasRealTransactions([{ ...correction('c2', 'a1'), description }] as never)).toBe(false)
  }
})

it('reads a plain key as done and a ! key as skipped', () => {
  expect(storedStepState('import,!plan', 'import')).toBe('done')
  expect(storedStepState('import,!plan', 'plan')).toBe('skipped')
  expect(storedStepState('import,!plan', 'accounts')).toBe('open')
  expect(storedStepState(null, 'accounts')).toBe('open')
})

it('withStepState replaces an earlier answer for the same key', () => {
  expect(withStepState('', 'plan', 'done')).toBe('plan')
  expect(withStepState('!plan,import', 'plan', 'done')).toBe('import,plan')
  expect(withStepState(null, 'import', 'skipped')).toBe('!import')
})

it('markGettingStartedDone sends nothing outside the flow or once answered', async () => {
  const posts: unknown[] = []
  server.use(http.post('*/api/v1/user/update-getting-started', async ({ request }) => {
    posts.push(await request.json())
    return HttpResponse.json({ success: true, message: '', data: { user: userWith('plan') } })
  }))
  const queryClient = new QueryClient()
  queryClient.setQueryData(queryKeys.user, userWith(null))
  markGettingStartedDone(queryClient, 'plan')
  queryClient.setQueryData(queryKeys.user, userWith('!plan'))
  markGettingStartedDone(queryClient, 'plan')
  queryClient.setQueryData(queryKeys.user, userWith(''))
  markGettingStartedDone(queryClient, 'plan')
  markGettingStartedDone(queryClient, 'plan')
  await waitFor(() => expect(posts).toEqual([{ value: 'plan' }]))
})
