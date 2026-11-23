import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as userApi from '@/api/user'
import { getUserData } from '@/api/user'
import { UserOptions } from '@/api/dto/user'
import type { CurrentUserDto } from '@/api/dto/user'
import type { Id } from '@/api/types'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'
import { rememberAnalyticsPreference } from '@/lib/analyticsPreference'
import { getBillingUrl } from '@/lib/config'
import { accessDaysLeft, deriveAccessState } from '@/lib/access'
import { toastWriteError } from '@/lib/apiError'
import type { AccessState } from '@/lib/access'

export function useUserData() {
  return useQuery({ queryKey: queryKeys.user, queryFn: getUserData, staleTime: TEN_MINUTES })
}

export interface AccessStateView {
  state: AccessState | undefined
  accessUntil: string
  daysLeft: number | null
  billingEnabled: boolean
}

export function useAccessState(): AccessStateView {
  const { data: user } = useUserData()
  const billingEnabled = getBillingUrl() !== ''
  if (!user) {
    return { state: undefined, accessUntil: '', daysLeft: null, billingEnabled }
  }
  const state = deriveAccessState(user.accessLevel, user.accessUntil)
  return {
    state,
    accessUntil: user.accessUntil,
    daysLeft: state === 'trial' ? accessDaysLeft(user.accessUntil) : null,
    billingEnabled,
  }
}

export function userOption(user: CurrentUserDto | undefined, name: UserOptions): string | null {
  return user?.options.find((o) => o.name === name)?.value ?? null
}

export function userCurrencyId(user: CurrentUserDto | undefined): string | null {
  return userOption(user, UserOptions.CURRENCY_ID)
}

// Vue quirk preserved: an ABSENT onboarding option means completed — only an
// explicit non-'completed' value routes the user into onboarding.
export function isOnboardingCompleted(user: CurrentUserDto | undefined): boolean {
  if (!user) {
    return false
  }
  const option = user.options.find((o) => o.name === UserOptions.ONBOARDING)
  return option === undefined || option.value === 'completed'
}

export function useUpdateName() {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { handlesOwnErrors: true },
    mutationFn: (name: string) => userApi.updateName(name),
    onSuccess: (user) => {
      queryClient.setQueryData(queryKeys.user, user)
      trackEvent(METRICS.USER_UPDATE_NAME)
    },
  })
}

export function useUpdateAvatar() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ icon, color }: { icon: string; color: string }) => userApi.updateAvatar(icon, color),
    onSuccess: (user) => {
      queryClient.setQueryData(queryKeys.user, user)
      // The avatar is denormalized into other payloads (transaction authors,
      // connections, account access), so every cached list may hold the old
      // value — refetch them all; avatar changes are rare enough.
      void queryClient.invalidateQueries()
      trackEvent(METRICS.USER_UPDATE_AVATAR)
    },
  })
}

export function useUpdatePassword() {
  return useMutation({
    meta: { handlesOwnErrors: true },
    mutationFn: ({ oldPassword, newPassword }: { oldPassword: string; newPassword: string }) =>
      userApi.updatePassword(oldPassword, newPassword),
    onSuccess: () => trackEvent(METRICS.USER_UPDATE_PASSWORD),
  })
}

export function useRequestEmailChange() {
  return useMutation({
    meta: { handlesOwnErrors: true },
    mutationFn: ({ newEmail, password }: { newEmail: string; password: string }) =>
      userApi.requestEmailChange(newEmail, password),
  })
}

export function useConfirmEmailChange() {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { handlesOwnErrors: true },
    mutationFn: ({ code }: { code: string }) => userApi.confirmEmailChange(code),
    onSuccess: (user) => {
      queryClient.setQueryData(queryKeys.user, user)
      trackEvent(METRICS.USER_CHANGE_EMAIL)
    },
  })
}

export function useResendEmailChangeCode() {
  return useMutation({
    meta: { handlesOwnErrors: true },
    mutationFn: () => userApi.resendEmailChangeCode(),
  })
}

export function useUpdateCurrency() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ currency }: { currency: string }) => userApi.updateCurrency(currency),
    onSuccess: (user) => {
      queryClient.setQueryData(queryKeys.user, user)
      trackEvent(METRICS.USER_UPDATE_CURRENCY)
    },
  })
}

export function useCompleteOnboarding() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => userApi.completeOnboarding(),
    onSuccess: (user) => {
      queryClient.setQueryData(queryKeys.user, user)
      trackEvent(METRICS.USER_COMPLETE_ONBOARDING)
    },
  })
}

export function useUpdateAnalytics() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (enabled: boolean) => {
      // Fired before the write so an opting-out user's last event still lands;
      // opting in is tracked after, once events are allowed again.
      if (!enabled) {
        trackEvent(METRICS.USER_UPDATE_ANALYTICS, { enabled })
      }
      const user = await userApi.updateAnalytics(enabled)
      rememberAnalyticsPreference(enabled)
      if (enabled) {
        trackEvent(METRICS.USER_UPDATE_ANALYTICS, { enabled })
      }
      return user
    },
    onSuccess: (user) => {
      queryClient.setQueryData(queryKeys.user, user)
    },
  })
}

// AI features are on unless the user turned them off (an absent option reads as on).
export function userAiOn(user: CurrentUserDto | undefined): boolean {
  return userOption(user, UserOptions.AI) !== '0'
}

// Whether AI runs for the user now: their option on and full access. A readonly user is treated
// as AI off without changing the stored option, which the Profile switch keeps showing.
export function userAiActive(user: CurrentUserDto | undefined): boolean {
  return user?.accessLevel !== 'readonly' && userAiOn(user)
}

export function useUpdateAi() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (enabled: boolean) => userApi.updateAi(enabled),
    onSuccess: (user, enabled) => {
      queryClient.setQueryData(queryKeys.user, user)
      // the server drops an opted-out user's open suggestions on the next read
      void queryClient.invalidateQueries({ queryKey: queryKeys.importSuggestions })
      void queryClient.invalidateQueries({ queryKey: queryKeys.importTransactionSuggestions })
      trackEvent(METRICS.USER_UPDATE_AI, { enabled })
    },
  })
}

// The import suggestion switches are on unless turned off, like the main AI switch.
export function userAiSuggestRules(user: CurrentUserDto | undefined): boolean {
  return userOption(user, UserOptions.AI_SUGGEST_RULES) !== '0'
}

export function userAiSuggestFill(user: CurrentUserDto | undefined): boolean {
  return userOption(user, UserOptions.AI_SUGGEST_FILL) !== '0'
}

function withOption(user: CurrentUserDto, name: UserOptions, value: string): CurrentUserDto {
  const options = user.options.some((o) => o.name === name)
    ? user.options.map((o) => (o.name === name ? { ...o, value } : o))
    : [...user.options, { name, value }]
  return { ...user, options }
}

const aiSuggestionOptions = [UserOptions.AI_SUGGEST_RULES, UserOptions.AI_SUGGEST_FILL]

// Only the two switch rows: a whole user from this write could revert another write in between,
// such as the main AI switch.
function withAiSuggestionOptions(current: CurrentUserDto | undefined, from: CurrentUserDto): CurrentUserDto {
  if (!current) return from
  return aiSuggestionOptions.reduce((user, name) => {
    const value = userOption(from, name)
    // absent reads as on, like the row it replaces
    return value === null ? { ...user, options: user.options.filter((o) => o.name !== name) } : withOption(user, name, value)
  }, current)
}

const updateAiSuggestionsKey = ['user', 'update-ai-suggestions'] as const

// Each switch posts both values, taking the other from the cached user, so the cache changes at
// once (onMutate runs at the click, even for a queued write): a second toggle before the first
// answer must read the first one's value. The writes are queued in one scope so the server
// commits them in click order and the latest toggle wins.
export function useUpdateAiSuggestions() {
  const queryClient = useQueryClient()
  // the mutation calling this still counts itself; a queued one counts too
  const othersPending = () => queryClient.isMutating({ mutationKey: updateAiSuggestionsKey }) > 1
  return useMutation({
    mutationKey: updateAiSuggestionsKey,
    scope: { id: 'user-update-ai-suggestions' },
    mutationFn: (body: { suggestRules: boolean; suggestFill: boolean }) => userApi.updateAiSuggestions(body),
    onMutate: async ({ suggestRules, suggestFill }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.user })
      const previous = queryClient.getQueryData<CurrentUserDto>(queryKeys.user)
      if (previous) {
        const next = withOption(withOption(previous, UserOptions.AI_SUGGEST_RULES, suggestRules ? '1' : '0'), UserOptions.AI_SUGGEST_FILL, suggestFill ? '1' : '0')
        queryClient.setQueryData(queryKeys.user, next)
      }
      return { previous }
    },
    // While a later toggle is queued, its answer settles the cache: it carries the full state.
    // Having an onError of its own, this write gets no global no-response toast: the switch
    // flips back, which must not happen silently.
    onError: (err, _vars, context) => {
      if (othersPending()) return
      const previous = context?.previous
      if (previous) queryClient.setQueryData<CurrentUserDto>(queryKeys.user, (current) => withAiSuggestionOptions(current, previous))
      void queryClient.invalidateQueries({ queryKey: queryKeys.user })
      toastWriteError(err)
    },
    onSuccess: (user, { suggestRules, suggestFill }) => {
      if (!othersPending()) queryClient.setQueryData<CurrentUserDto>(queryKeys.user, (current) => withAiSuggestionOptions(current, user))
      void queryClient.invalidateQueries({ queryKey: queryKeys.importSuggestions })
      void queryClient.invalidateQueries({ queryKey: queryKeys.importTransactionSuggestions })
      trackEvent(METRICS.USER_AI_SUGGESTIONS_UPDATE, { rules: suggestRules, fill: suggestFill })
    },
  })
}

export function useUpdateLastSeenVersion() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (version: string) => userApi.updateLastSeenVersion(version),
    onSuccess: (user) => {
      queryClient.setQueryData(queryKeys.user, user)
    },
  })
}

export function useUpdateDefaultBudget() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (budgetId: Id) => userApi.updateDefaultBudget(budgetId),
    onSuccess: (user) => {
      queryClient.setQueryData(queryKeys.user, user)
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      trackEvent(METRICS.USER_UPDATE_DEFAULT_BUDGET)
    },
  })
}
