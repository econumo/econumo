import { useMutation } from '@tanstack/react-query'
import * as userApi from '@/api/user'
import { setToken } from '@/lib/storage'
import { METRICS, trackEvent } from '@/lib/metrics'

export function useLogin() {
  return useMutation({
    mutationFn: ({ username, password }: { username: string; password: string }) =>
      userApi.login(username, password),
    onSuccess: (data) => {
      setToken(data.token)
      trackEvent(METRICS.USER_LOGIN)
    },
  })
}

export function useRegister() {
  return useMutation({
    mutationFn: ({ email, password, name }: { email: string; password: string; name: string }) =>
      userApi.register(email, password, name),
  })
}

export function useRemindPassword() {
  return useMutation({
    mutationFn: ({ username }: { username: string }) => userApi.remindPassword(username),
  })
}

export function useResetPassword() {
  return useMutation({
    mutationFn: ({ username, code, password }: { username: string; code: string; password: string }) =>
      userApi.resetPassword(username, code, password),
  })
}
