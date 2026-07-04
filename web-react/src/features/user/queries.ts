import { useQuery } from '@tanstack/react-query'
import { getUserData } from '@/api/user'
import { UserOptions } from '@/api/dto/user'
import type { CurrentUserDto } from '@/api/dto/user'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'

export function useUserData() {
  return useQuery({ queryKey: queryKeys.user, queryFn: getUserData, staleTime: TEN_MINUTES })
}

export function userOption(user: CurrentUserDto | undefined, name: UserOptions): string | null {
  return user?.options.find((o) => o.name === name)?.value ?? null
}

export function userCurrencyId(user: CurrentUserDto | undefined): string | null {
  return userOption(user, UserOptions.CURRENCY_ID)
}

export function isOnboardingCompleted(user: CurrentUserDto | undefined): boolean {
  return userOption(user, UserOptions.ONBOARDING) === 'completed'
}
