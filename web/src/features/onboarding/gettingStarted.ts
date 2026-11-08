import type { QueryClient } from '@tanstack/react-query'
import { UserOptions, type CurrentUserDto } from '@/api/dto/user'
import type { TransactionDto } from '@/api/dto/transaction'
import { updateGettingStarted } from '@/api/user'
import { queryKeys } from '@/app/queryKeys'
import { gettingStartedOption } from '@/features/user/queries'

export type StepKey = 'home_screen' | 'apple_wallet' | 'plan' | 'import'
// the steps whose state lives in the users_options value; add to home screen is per device
export type StoredStepKey = Exclude<StepKey, 'home_screen'>

export type StepState = 'open' | 'done' | 'dismissed'

export function storedStepState(option: string | null, key: StoredStepKey): StepState {
  const keys = (option ?? '').split(',')
  if (keys.includes(key)) return 'done'
  if (keys.includes(`!${key}`)) return 'dismissed'
  return 'open'
}

// The server refuses a key listed twice, so a new state replaces the old one.
export function withStepState(option: string | null, key: StoredStepKey, state: 'done' | 'dismissed'): string {
  const rest = (option ?? '').split(',').filter((k) => k !== '' && k !== key && k !== `!${key}`)
  return [...rest, state === 'done' ? key : `!${key}`].join(',')
}

let writesInFlight = 0

// The value is written into the cached user before the request, so two quick
// answers build on each other instead of the second overwriting the first. A
// user fetch already running would put the old value back, so it is cancelled
// first; one started meanwhile (a CSV import refetches everything) is settled
// by refetching once the last write lands, which also cancels it. Only the
// last write reconciles, so overlapping writes do not flicker.
async function writeStepState(queryClient: QueryClient, key: StoredStepKey, state: 'done' | 'dismissed'): Promise<void> {
  await queryClient.cancelQueries({ queryKey: queryKeys.user })
  const user = queryClient.getQueryData<CurrentUserDto>(queryKeys.user)
  const option = gettingStartedOption(user)
  if (!user || option === null || storedStepState(option, key) === state) return
  const value = withStepState(option, key, state)
  queryClient.setQueryData<CurrentUserDto>(queryKeys.user, {
    ...user,
    options: user.options.map((o) => (o.name === UserOptions.GETTING_STARTED ? { ...o, value } : o)),
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
  if (storedStepState(gettingStartedOption(queryClient.getQueryData<CurrentUserDto>(queryKeys.user)), key) !== 'open') return
  void writeStepState(queryClient, key, 'done')
}

export function dismissGettingStarted(queryClient: QueryClient, key: StoredStepKey): void {
  void writeStepState(queryClient, key, 'dismissed')
}

// The descriptions the server stores, in English, on the corrections it
// writes itself (internal/account and internal/transaction usecase.go).
const CORRECTION_DESCRIPTIONS = new Set([
  'Balance adjustment',
  'Balance adjustment (account deleted)',
  'Balance adjustment (deleted transaction)',
  'Balance adjustment (edited transaction)',
])

// An opening balance, or a correction the server wrote (a balance edit, a
// restored balance), may be one of several on an account. Neither is the
// user's own spending, so neither ends the get-ready phase; a bare row the
// user described themselves ("Coffee") is.
function isCorrection(t: Pick<TransactionDto, 'type' | 'accountRecipientId' | 'categoryId' | 'payeeId' | 'tagId' | 'description' | 'labelIds' | 'recurringId'>): boolean {
  return (
    t.type !== 'transfer' &&
    !t.accountRecipientId &&
    !t.categoryId &&
    !t.payeeId &&
    !t.tagId &&
    !t.recurringId &&
    (t.labelIds ?? []).length === 0 &&
    (t.description === '' || CORRECTION_DESCRIPTIONS.has(t.description))
  )
}

export function hasRealTransactions(transactions: TransactionDto[]): boolean {
  return transactions.some((t) => !isCorrection(t))
}
