import type { QueryClient } from '@tanstack/react-query'
import { UserOptions, type CurrentUserDto } from '@/api/dto/user'
import { updateOnboarding } from '@/api/user'
import { queryKeys } from '@/app/queryKeys'
import { isOnboardingCompleted, userOption } from '@/features/user/queries'

export type OnboardingStepKey = 'import' | 'categories' | 'accounts' | 'plan' | 'home_screen' | 'apple_wallet'
// the steps whose state lives in the onboarding option; add to home screen is per device
export type StoredStepKey = Exclude<OnboardingStepKey, 'home_screen'>

export type OnboardingStepState = 'open' | 'done' | 'skipped'
type Answer = Exclude<OnboardingStepState, 'open'>

export function storedStepState(option: string | null, key: StoredStepKey): OnboardingStepState {
  const keys = (option ?? '').split(',')
  if (keys.includes(key)) return 'done'
  if (keys.includes(`!${key}`)) return 'skipped'
  return 'open'
}

// The server refuses a key listed twice, so a new state replaces the old one.
export function withStepState(option: string | null, key: StoredStepKey, state: Answer): string {
  const rest = (option ?? '').split(',').filter((k) => k !== '' && k !== key && k !== `!${key}`)
  return [...rest, state === 'done' ? key : `!${key}`].join(',')
}

const ONBOARDING_STARTED = 'started'

// While onboarding is in progress the onboarding option holds "started"
// (nothing answered) or the answered step list; "completed" (or an absent
// option, a user predating onboarding) is terminal: null, nothing is written.
export function onboardingProgress(user: CurrentUserDto | undefined): string | null {
  if (user === undefined || isOnboardingCompleted(user)) return null
  const value = userOption(user, UserOptions.ONBOARDING)
  return value === null || value === ONBOARDING_STARTED ? '' : value
}

let writesInFlight = 0

// The value is written into the cached user before the request, so two quick
// answers build on each other instead of the second overwriting the first. A
// user fetch already running would put the old value back, so it is cancelled
// first; one started meanwhile (a CSV import refetches everything) is settled
// by refetching once the last write lands, which also cancels it. Only the
// last write reconciles, so overlapping writes do not flicker.
export async function writeStepState(queryClient: QueryClient, key: StoredStepKey, state: Answer): Promise<void> {
  await queryClient.cancelQueries({ queryKey: queryKeys.user })
  const user = queryClient.getQueryData<CurrentUserDto>(queryKeys.user)
  const option = onboardingProgress(user)
  if (!user || option === null || storedStepState(option, key) === state) return
  const value = withStepState(option, key, state)
  queryClient.setQueryData<CurrentUserDto>(queryKeys.user, {
    ...user,
    options: user.options.map((o) => (o.name === UserOptions.ONBOARDING ? { ...o, value } : o)),
  })
  writesInFlight++
  try {
    const saved = await updateOnboarding(value)
    if (writesInFlight === 1) queryClient.setQueryData(queryKeys.user, saved)
  } catch {
    // the refetch below restores the stored value
  } finally {
    writesInFlight--
    if (writesInFlight === 0) void queryClient.invalidateQueries({ queryKey: queryKeys.user })
  }
}

// Callable from the budget mutations without a hook of their own: nothing is
// sent outside the flow or once the step has an answer.
export function markOnboardingStepDone(queryClient: QueryClient, key: StoredStepKey): void {
  const option = onboardingProgress(queryClient.getQueryData<CurrentUserDto>(queryKeys.user))
  if (option === null || storedStepState(option, key) !== 'open') return
  void writeStepState(queryClient, key, 'done')
}
