import type { QueryClient } from '@tanstack/react-query'
import { UserOptions, type CurrentUserDto } from '@/api/dto/user'
import { updateGettingStarted } from '@/api/user'
import { queryKeys } from '@/app/queryKeys'
import { gettingStartedOption, isOnboardingCompleted } from '@/features/user/queries'

export type OnboardingStepKey = 'import' | 'categories' | 'accounts' | 'plan' | 'home_screen' | 'apple_wallet'
// the steps whose state lives in the users_options value; add to home screen is per device
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

// Registration seeds no getting_started option, so for a user still in the
// flow an absent option reads as "nothing answered yet". Once onboarding is
// complete an absent option means the user predates the stepper: null, and
// nothing is ever written for them.
export function flowOption(user: CurrentUserDto | undefined): string | null {
  const option = gettingStartedOption(user)
  if (option !== null) return option
  return user !== undefined && !isOnboardingCompleted(user) ? '' : null
}

let writesInFlight = 0

// The value is written into the cached user before the request, so two quick
// answers build on each other instead of the second overwriting the first. A
// user fetch already running would put the old value back, so it is cancelled
// first; one started meanwhile (a CSV import refetches everything) is settled
// by refetching once the last write lands, which also cancels it. Only the
// last write reconciles, so overlapping writes do not flicker.
export async function writeStepState(queryClient: QueryClient, key: StoredStepKey, state: Answer): Promise<void> {
  await writeStepStates(queryClient, [key], state)
}

// Several keys answered in one option write (a CSV import settles three steps).
export async function markStepsDone(queryClient: QueryClient, keys: StoredStepKey[]): Promise<void> {
  await writeStepStates(queryClient, keys, 'done')
}

async function writeStepStates(queryClient: QueryClient, keys: StoredStepKey[], state: Answer): Promise<void> {
  await queryClient.cancelQueries({ queryKey: queryKeys.user })
  const user = queryClient.getQueryData<CurrentUserDto>(queryKeys.user)
  const option = flowOption(user)
  if (!user || option === null) return
  const changing = keys.filter((key) => storedStepState(option, key) !== state)
  if (changing.length === 0) return
  const value = changing.reduce((acc, key) => withStepState(acc, key, state), option)
  const stored = user.options.some((o) => o.name === UserOptions.GETTING_STARTED)
  queryClient.setQueryData<CurrentUserDto>(queryKeys.user, {
    ...user,
    options: stored
      ? user.options.map((o) => (o.name === UserOptions.GETTING_STARTED ? { ...o, value } : o))
      : [...user.options, { name: UserOptions.GETTING_STARTED, value }],
  })
  writesInFlight++
  try {
    const saved = await updateGettingStarted(value)
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
export function markGettingStartedDone(queryClient: QueryClient, key: StoredStepKey): void {
  const option = flowOption(queryClient.getQueryData<CurrentUserDto>(queryKeys.user))
  if (option === null || storedStepState(option, key) !== 'open') return
  void writeStepState(queryClient, key, 'done')
}
