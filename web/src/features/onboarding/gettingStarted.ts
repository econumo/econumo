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

// Written straight into the cached user before the request, so two quick
// answers build on each other instead of the second overwriting the first;
// the response is not applied for the same reason. A failed write refetches.
function writeStepState(queryClient: QueryClient, key: StoredStepKey, state: 'done' | 'dismissed'): void {
  const user = queryClient.getQueryData<CurrentUserDto>(queryKeys.user)
  const option = gettingStartedOption(user)
  if (!user || option === null || storedStepState(option, key) === state) return
  const value = withStepState(option, key, state)
  queryClient.setQueryData<CurrentUserDto>(queryKeys.user, {
    ...user,
    options: user.options.map((o) => (o.name === UserOptions.GETTING_STARTED ? { ...o, value } : o)),
  })
  updateGettingStarted(value).catch(() => queryClient.invalidateQueries({ queryKey: queryKeys.user }))
}

// Callable from the budget mutations without a hook of their own: nothing is
// sent outside the flow or once the step has an answer.
export function markGettingStartedDone(queryClient: QueryClient, key: StoredStepKey): void {
  if (storedStepState(gettingStartedOption(queryClient.getQueryData<CurrentUserDto>(queryKeys.user)), key) !== 'open') return
  writeStepState(queryClient, key, 'done')
}

export function dismissGettingStarted(queryClient: QueryClient, key: StoredStepKey): void {
  writeStepState(queryClient, key, 'dismissed')
}

// The same shape the server treats as an untouched opening correction: the
// account's only transaction, carrying nothing but an amount and a date.
function isBareCorrection(t: Pick<TransactionDto, 'type' | 'accountRecipientId' | 'categoryId' | 'payeeId' | 'tagId' | 'description' | 'labelIds' | 'recurringId'>): boolean {
  return (
    t.type !== 'transfer' &&
    !t.accountRecipientId &&
    !t.categoryId &&
    !t.payeeId &&
    !t.tagId &&
    !t.recurringId &&
    t.description === '' &&
    (t.labelIds ?? []).length === 0
  )
}

export function hasRealTransactions(transactions: TransactionDto[]): boolean {
  const corrected = new Set<string>()
  for (const t of transactions) {
    if (!isBareCorrection(t) || corrected.has(t.accountId)) return true
    corrected.add(t.accountId)
  }
  return false
}
