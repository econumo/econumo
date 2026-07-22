import { useMutation } from '@tanstack/react-query'
import * as userApi from '@/api/user'
import { clearPersistedQueryCache } from '@/lib/queryPersist'
import { setToken } from '@/lib/storage'
import { METRICS, trackEvent } from '@/lib/metrics'

export function useLogin() {
  return useMutation({
    mutationFn: ({ username, password }: { username: string; password: string }) => userApi.login(username, password),
    onSuccess: (data) => {
      // the new session may belong to a different user — never restore the
      // previous user's persisted finances
      clearPersistedQueryCache()
      setToken(data.token)
      trackEvent(METRICS.USER_LOGIN)
    },
  })
}

export function useConfirmEmail() {
  return useMutation({
    mutationFn: ({ username, code }: { username: string; code: string }) => userApi.confirmEmail(username, code),
    onSuccess: () => trackEvent(METRICS.EMAIL_VERIFICATION_COMPLETED),
  })
}

export function useResendVerification() {
  return useMutation({
    mutationFn: ({ username }: { username: string }) => userApi.resendVerificationCode(username),
    onSuccess: () => trackEvent(METRICS.EMAIL_VERIFICATION_RESENT),
  })
}

export function useRegister() {
  return useMutation({
    mutationFn: ({ email, password, name }: { email: string; password: string; name: string }) =>
      userApi.register(email, password, name),
    onSuccess: () => trackEvent(METRICS.USER_REGISTRATION),
  })
}

export function useRemindPassword() {
  return useMutation({
    mutationFn: ({ username }: { username: string }) => userApi.remindPassword(username),
    onSuccess: () => trackEvent(METRICS.USER_REMIND_PASSWORD),
  })
}

export function useResetPassword() {
  return useMutation({
    mutationFn: ({ username, code, password }: { username: string; code: string; password: string }) =>
      userApi.resetPassword(username, code, password),
    onSuccess: () => trackEvent(METRICS.USER_RESET_PASSWORD),
  })
}
