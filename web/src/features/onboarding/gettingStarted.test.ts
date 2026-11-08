import { waitFor } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { fixtureUser } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { flowOption, markGettingStartedDone, storedStepState, withStepState, writeStepState } from './gettingStarted'

const userWith = (value: string | null, onboarding = 'completed') => ({
  ...fixtureUser,
  options: [
    ...fixtureUser.options.map((o) => (o.name === 'onboarding' ? { ...o, value: onboarding } : o)),
    ...(value === null ? [] : [{ name: 'getting_started', value }]),
  ],
})

// what registration stores: onboarding=started and no getting_started option
const registeredUser = () => userWith(null, 'started')

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

it('flowOption reads an absent option as empty only while onboarding is not complete', () => {
  expect(flowOption(registeredUser() as never)).toBe('')
  expect(flowOption(userWith(null) as never)).toBeNull()
  expect(flowOption(userWith('plan') as never)).toBe('plan')
  expect(flowOption(undefined)).toBeNull()
})

it('a registered user without the option gets it written', async () => {
  const posts: unknown[] = []
  server.use(
    http.post('*/api/v1/user/update-getting-started', async ({ request }) => {
      const body = (await request.json()) as { value: string }
      posts.push(body)
      return HttpResponse.json({ success: true, message: '', data: { user: userWith(body.value, 'started') } })
    }),
    http.get('*/api/v1/user/get-user-data', () => HttpResponse.json({ success: true, message: '', data: { user: userWith('!import', 'started') } })),
  )
  const queryClient = new QueryClient()
  queryClient.setQueryData(queryKeys.user, registeredUser())
  await writeStepState(queryClient, 'import', 'skipped')
  expect(posts).toEqual([{ value: '!import' }])
  expect(flowOption(queryClient.getQueryData(queryKeys.user))).toBe('!import')
})

it('markGettingStartedDone writes for a registered user without the option', async () => {
  const posts: unknown[] = []
  server.use(http.post('*/api/v1/user/update-getting-started', async ({ request }) => {
    posts.push(await request.json())
    return HttpResponse.json({ success: true, message: '', data: { user: userWith('plan', 'started') } })
  }))
  const queryClient = new QueryClient()
  queryClient.setQueryData(queryKeys.user, registeredUser())
  markGettingStartedDone(queryClient, 'plan')
  await waitFor(() => expect(posts).toEqual([{ value: 'plan' }]))
})
