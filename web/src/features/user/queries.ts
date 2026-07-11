import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as userApi from '@/api/user'
import { getUserData } from '@/api/user'
import { UserOptions } from '@/api/dto/user'
import type { CurrentUserDto } from '@/api/dto/user'
import type { Id } from '@/api/types'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'

export function useUserData() {
  return useQuery({ queryKey: queryKeys.user, queryFn: getUserData, staleTime: TEN_MINUTES })
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
    mutationFn: ({ oldPassword, newPassword }: { oldPassword: string; newPassword: string }) =>
      userApi.updatePassword(oldPassword, newPassword),
    onSuccess: () => trackEvent(METRICS.USER_UPDATE_PASSWORD),
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
