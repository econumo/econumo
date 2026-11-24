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
import { accessDaysLeft, deriveAccessState, isFutureUtc, isRenewing } from '@/lib/access'
import type { AccessState } from '@/lib/access'

export function useUserData() {
  return useQuery({ queryKey: queryKeys.user, queryFn: getUserData, staleTime: TEN_MINUTES })
}

// The payment portal's view of the subscription, asked only where billing is
// configured. Without an answer (billing off, an older portal, a failure) the
// server reports available: false and the app behaves as it always has.
export function useBillingAccess() {
  return useQuery({
    queryKey: queryKeys.billingAccess,
    queryFn: userApi.getBillingAccess,
    staleTime: 5 * 60_000,
    enabled: getBillingUrl() !== '',
  })
}

export interface AccessStateView {
  state: AccessState | undefined
  accessUntil: string
  // null while renewing: a renewing subscription counts down to nothing
  daysLeft: number | null
  // billing's view: a subscription still billing; renewsAt is its next
  // renewal while that is ahead ('' otherwise); endsAt a scheduled end
  renewing: boolean
  renewsAt: string
  endsAt: string
  // false while billing's view is still loading: ending warnings wait for it,
  // or a renewing subscriber would see one flash
  billingSettled: boolean
  billingEnabled: boolean
}

export function useAccessState(): AccessStateView {
  const { data: user } = useUserData()
  const { data: billing, isPending: billingPending } = useBillingAccess()
  const billingEnabled = getBillingUrl() !== ''
  const billingSettled = !billingEnabled || !billingPending
  if (!user) {
    return { state: undefined, accessUntil: '', daysLeft: null, renewing: false, renewsAt: '', endsAt: '', billingSettled, billingEnabled }
  }
  const state = deriveAccessState(user.accessLevel, user.accessUntil)
  const billingUser = billingEnabled && billing?.available ? billing.user : null
  const renewing = state !== 'readonly' && isRenewing(billingUser)
  return {
    state,
    accessUntil: user.accessUntil,
    daysLeft: state === 'trial' && !renewing ? accessDaysLeft(user.accessUntil) : null,
    renewing,
    renewsAt: renewing && isFutureUtc(billingUser!.renewsAt) ? billingUser!.renewsAt : '',
    endsAt: state !== 'readonly' && !renewing && billingUser?.endsAt ? billingUser.endsAt : '',
    billingSettled,
    billingEnabled,
  }
}

// Ids of the connections billing reports as renewing subscribers.
export function useRenewingConnectionIds(): ReadonlySet<string> {
  const { data: billing } = useBillingAccess()
  if (getBillingUrl() === '' || !billing?.available) return new Set()
  return new Set(billing.connections.filter((c) => c.renewing).map((c) => c.id))
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
      trackEvent(METRICS.USER_UPDATE_AI, { enabled })
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
