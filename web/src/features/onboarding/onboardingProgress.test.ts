import { waitFor } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { fixtureUser } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { markOnboardingStepDone, onboardingProgress, storedStepState, withStepState, writeStepState } from './onboardingProgress'

const userWith = (onboarding: string | null) => ({
  ...fixtureUser,
  options: onboarding === null
    ? fixtureUser.options.filter((o) => o.name !== 'onboarding')
    : fixtureUser.options.map((o) => (o.name === 'onboarding' ? { ...o, value: onboarding } : o)),
})

// what registration stores: onboarding=started
const registeredUser = () => userWith('started')

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

it('markOnboardingStepDone sends nothing once onboarding is complete or the step is answered', async () => {
  const posts: unknown[] = []
  server.use(http.post('*/api/v1/user/update-onboarding', async ({ request }) => {
    posts.push(await request.json())
    return HttpResponse.json({ success: true, message: '', data: { user: userWith('plan') } })
  }))
  const queryClient = new QueryClient()
  queryClient.setQueryData(queryKeys.user, userWith('completed'))
  markOnboardingStepDone(queryClient, 'plan')
  queryClient.setQueryData(queryKeys.user, userWith(null))
  markOnboardingStepDone(queryClient, 'plan')
  queryClient.setQueryData(queryKeys.user, userWith('!plan'))
  markOnboardingStepDone(queryClient, 'plan')
  queryClient.setQueryData(queryKeys.user, registeredUser())
  markOnboardingStepDone(queryClient, 'plan')
  markOnboardingStepDone(queryClient, 'plan')
  await waitFor(() => expect(posts).toEqual([{ value: 'plan' }]))
})

it('onboardingProgress reads "started" as nothing answered and is null once complete', () => {
  expect(onboardingProgress(registeredUser() as never)).toBe('')
  expect(onboardingProgress(userWith('completed') as never)).toBeNull()
  expect(onboardingProgress(userWith(null) as never)).toBeNull()
  expect(onboardingProgress(userWith('plan') as never)).toBe('plan')
  expect(onboardingProgress(undefined)).toBeNull()
})

it('a registered user answering a step writes the step list into the onboarding option', async () => {
  const posts: unknown[] = []
  server.use(
    http.post('*/api/v1/user/update-onboarding', async ({ request }) => {
      const body = (await request.json()) as { value: string }
      posts.push(body)
      return HttpResponse.json({ success: true, message: '', data: { user: userWith(body.value) } })
    }),
    http.get('*/api/v1/user/get-user-data', () => HttpResponse.json({ success: true, message: '', data: { user: userWith('!import') } })),
  )
  const queryClient = new QueryClient()
  queryClient.setQueryData(queryKeys.user, registeredUser())
  await writeStepState(queryClient, 'import', 'skipped')
  expect(posts).toEqual([{ value: '!import' }])
  expect(onboardingProgress(queryClient.getQueryData(queryKeys.user))).toBe('!import')
})
