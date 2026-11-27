import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { isAxiosError } from 'axios'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { LoadingDialog } from '@/components/LoadingDialog'
import { useOnline } from '@/hooks/useOnline'
import { apiErrorMessage, apiFieldErrors } from '@/lib/apiError'
import { getBillingUrl } from '@/lib/config'
import { downloadBlob, localDateStamp } from '@/lib/download'
import { isNotEmpty, isValidRecoveryCode } from '@/lib/validation'
import { DELETION_CLEANUP_KEY } from '@/api/client'
import { exportTransactionList } from '@/api/transaction'
import type { Id } from '@/api/types'
import { RouterPage } from '@/app/router-pages'
import { useOpenBillingPortal } from '@/features/access/useOpenBillingPortal'
import { useAccounts } from '@/features/accounts/queries'
import { signOutLocally } from '@/features/auth/localSignOut'
import { usePasswordLoginAvailable } from '@/features/auth/oauthQueries'
import { useWaitingCount } from '@/features/pendingWrites/hooks'
import { useConfirmUserDeletion, useRequestUserDeletion, useUserData } from '@/features/user/queries'
import { SettingsShell } from './SettingsShell'

type ExportState = 'idle' | 'running' | 'done' | 'failed' | 'none'

function setCleanupFlag(): void {
  try {
    localStorage.setItem(DELETION_CLEANUP_KEY, String(Date.now()))
  } catch {
    // without storage the interceptor cannot finish an interrupted clean-up
  }
}

function clearCleanupFlag(): void {
  try {
    localStorage.removeItem(DELETION_CLEANUP_KEY)
  } catch {
    // nothing stored
  }
}

export function DeleteDataPage() {
  const { t } = useTranslation()
  const online = useOnline()
  const { data: user } = useUserData()
  const { data: accounts, isSuccess: accountsLoaded } = useAccounts()
  const passwordLogin = usePasswordLoginAvailable()
  const askPassword = user?.hasPassword !== false && passwordLogin
  const waiting = useWaitingCount()
  const billingEnabled = getBillingUrl() !== ''
  const portal = useOpenBillingPortal()
  const requestDeletion = useRequestUserDeletion()
  const confirmDeletion = useConfirmUserDeletion()

  const [phase, setPhase] = useState<'request' | 'confirm'>('request')
  const [password, setPassword] = useState('')
  const [passwordError, setPasswordError] = useState<string | undefined>()
  const [requestError, setRequestError] = useState('')
  const [code, setCode] = useState('')
  const [codeError, setCodeError] = useState<string | null>(null)
  const [confirmError, setConfirmError] = useState('')
  const [confirming, setConfirming] = useState(false)
  const [exportState, setExportState] = useState<ExportState>('idle')
  const exportStarted = useRef(false)

  const userId = user?.id
  const ownedIds = useMemo(() => (accounts ?? []).filter((a) => a.owner.id === userId).map((a) => a.id), [accounts, userId])

  const download = useCallback(async (ids: Id[]) => {
    // a blank accountId means every accessible account to the server
    if (ids.length === 0) {
      setExportState('none')
      return
    }
    setExportState('running')
    try {
      const blob = await exportTransactionList(ids)
      downloadBlob(blob, `transactions-${localDateStamp()}.csv`)
      setExportState('done')
    } catch {
      setExportState('failed')
    }
  }, [])

  useEffect(() => {
    if (phase !== 'confirm' || !accountsLoaded || !userId || exportStarted.current) {
      return
    }
    exportStarted.current = true
    void download(ownedIds)
  }, [phase, accountsLoaded, userId, ownedIds, download])

  const submitRequest = () => {
    setRequestError('')
    if (askPassword && !isNotEmpty(password)) {
      setPasswordError(t('user.form.password.validation.required_field'))
      return
    }
    setPasswordError(undefined)
    requestDeletion.mutate(
      { password: askPassword ? password : '' },
      {
        onSuccess: () => setPhase('confirm'),
        onError: (err) => {
          const fieldError = apiFieldErrors(err, 'password')?.[0]
          setPasswordError(fieldError)
          setRequestError(fieldError ? '' : apiErrorMessage(err))
        },
      },
    )
  }

  const submitConfirm = async () => {
    setConfirmError('')
    const trimmed = code.trim()
    if (!isNotEmpty(trimmed)) {
      setCodeError(t('user.form.code.validation.required_field'))
      return
    }
    if (!isValidRecoveryCode(trimmed)) {
      setCodeError(t('user.form.code.validation.invalid_code'))
      return
    }
    setCodeError(null)
    setConfirming(true)
    // Set before sending: if this tab dies between the server's commit and the
    // response, the next 401 still clears what this browser holds.
    setCleanupFlag()
    try {
      await confirmDeletion.mutateAsync({ code: trimmed })
    } catch (err) {
      // Without a response the server may have committed, so the flag stays;
      // a 401 is left to the interceptor, which reads it.
      if (isAxiosError(err) && err.response && err.response.status !== 401) {
        clearCleanupFlag()
      }
      setConfirmError(apiErrorMessage(err))
      setConfirming(false)
      return
    }
    // the server has revoked every session: a closed tab must not leave the data behind
    await signOutLocally()
    clearCleanupFlag()
    window.location.assign('/login?reason=deletion-scheduled')
  }

  return (
    <SettingsShell
      title={t('user.delete_data.header')}
      backTo={RouterPage.SETTINGS_PROFILE}
      crumbs={[
        { label: t('settings.page.header_desktop'), to: RouterPage.SETTINGS },
        { label: t('user.page.settings.profile.menu_item'), to: RouterPage.SETTINGS_PROFILE },
      ]}
    >
      {phase === 'request' ? (
        <form
          className="flex max-w-md flex-col gap-4 py-2"
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            submitRequest()
          }}
        >
          <div className="flex flex-col gap-3 rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm">
            <p>{t('user.delete_data.explain.what')}</p>
            <p>{t('user.delete_data.explain.shared')}</p>
            <p>{t('user.delete_data.explain.cancel')}</p>
            {billingEnabled ? (
              <p>
                {t('user.delete_data.explain.subscription')}{' '}
                <button
                  type="button"
                  className="text-econumo-link underline disabled:opacity-50"
                  disabled={portal.disabled}
                  onClick={() => portal.open()}
                >
                  {t('user.delete_data.explain.billing_link')}
                </button>
              </p>
            ) : null}
          </div>
          {askPassword ? (
            <div className="flex flex-col gap-2">
              <Label htmlFor="dd-password">{t('user.delete_data.form.password.label')}</Label>
              <Input
                id="dd-password"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
              {passwordError ? <p className="text-sm text-destructive">{passwordError}</p> : null}
            </div>
          ) : null}
          {requestError ? <p className="text-sm text-destructive">{requestError}</p> : null}
          <Button type="submit" className="h-10 w-full font-normal lg:w-auto lg:min-w-44 lg:self-start" disabled={requestDeletion.isPending || !online}>
            {t('user.delete_data.form.submit.label')}
          </Button>
        </form>
      ) : (
        <form
          className="flex max-w-md flex-col gap-4 py-2"
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            void submitConfirm()
          }}
        >
          <p className="text-sm text-muted-foreground">{t('user.delete_data.confirm.information')}</p>
          {exportState === 'done' ? <p className="text-sm text-muted-foreground">{t('user.delete_data.confirm.export.done')}</p> : null}
          {exportState === 'none' ? <p className="text-sm text-muted-foreground">{t('user.delete_data.confirm.export.none')}</p> : null}
          {exportState === 'failed' ? (
            <div className="flex flex-col items-start gap-2">
              <p className="text-sm text-destructive">{t('user.delete_data.confirm.export.failed')}</p>
              <Button type="button" variant="secondary" onClick={() => void download(ownedIds)}>
                {t('user.delete_data.confirm.export.retry')}
              </Button>
            </div>
          ) : null}
          <div className="flex flex-col gap-2">
            <Label htmlFor="dd-code">{t('user.form.code.placeholder')}</Label>
            <Input
              id="dd-code"
              autoFocus
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
            {codeError ? <p className="text-sm text-destructive">{codeError}</p> : null}
          </div>
          {confirmError ? <p className="text-sm text-destructive">{confirmError}</p> : null}
          {waiting > 0 ? <p className="text-sm text-destructive">{t('user.delete_data.confirm.pending_writes')}</p> : null}
          <Button
            type="submit"
            variant="destructive"
            className="h-10 w-full font-normal lg:w-auto lg:min-w-44 lg:self-start"
            disabled={confirming || waiting > 0 || !online}
          >
            {t('user.delete_data.confirm.action')}
          </Button>
        </form>
      )}

      <LoadingDialog open={requestDeletion.isPending} label={t('user.change_email.loading.label')} />
    </SettingsShell>
  )
}
