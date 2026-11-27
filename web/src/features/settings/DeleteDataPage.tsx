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
import { DELETION_CLEANUP_KEY, deletionConfirmPending, expireSession, finishDeletionCleanup } from '@/api/client'
import { exportTransactionList } from '@/api/transaction'
import type { Id } from '@/api/types'
import { RouterPage } from '@/app/router-pages'
import { useOpenBillingPortal } from '@/features/access/useOpenBillingPortal'
import { useAccounts } from '@/features/accounts/queries'
import { signOutLocally } from '@/features/auth/localSignOut'
import { usePasswordLoginAvailable } from '@/features/auth/oauthQueries'
import { useStoredQueueSettled, useWaitingCount } from '@/features/pendingWrites/hooks'
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

// a blank accountId means every accessible account to the server, so only owned ids go out
function ownedAccountIds(accounts: { id: Id; owner: { id: Id } }[] | undefined, userId: Id | undefined): Id[] {
  return (accounts ?? []).filter((a) => a.owner.id === userId).map((a) => a.id)
}

export function DeleteDataPage() {
  const { t } = useTranslation()
  const online = useOnline()
  const { data: user } = useUserData()
  const accountsQuery = useAccounts()
  const { data: accounts, isSuccess: accountsLoaded } = accountsQuery
  const accountsFailed = accountsQuery.isError && accounts === undefined
  const passwordLogin = usePasswordLoginAvailable()
  const askPassword = user?.hasPassword !== false && passwordLogin
  // 0 means nothing until the stored queue is in memory; a store that gave up
  // counts as settled so Confirm cannot stay disabled for good
  const queueSettled = useStoredQueueSettled()
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
  const ownedIds = useMemo(() => ownedAccountIds(accounts, userId), [accounts, userId])
  // the CSV must hold every change, so it waits for the queue to be known and empty
  const exportReady = phase === 'confirm' && queueSettled && waiting === 0 && !!userId
  const shownExport: ExportState = exportState === 'idle' && exportReady && accountsFailed ? 'failed' : exportState

  const download = useCallback(async (ids: Id[]) => {
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
    if (!exportReady || !accountsLoaded || exportStarted.current) {
      return
    }
    exportStarted.current = true
    void download(ownedIds)
  }, [exportReady, accountsLoaded, ownedIds, download])

  const retryDownload = async () => {
    exportStarted.current = true
    setExportState('running')
    const res = await accountsQuery.refetch()
    if (res.data === undefined) {
      setExportState('failed')
      return
    }
    await download(ownedAccountIds(res.data, userId))
  }

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
    // Read from storage before this attempt writes it: a flag left by an earlier
    // attempt that got no response survives a reload, and means that attempt's
    // mark may have committed.
    const earlierAttemptPending = deletionConfirmPending()
    // Set before sending: if this tab dies between the server's commit and the
    // response, the next 401 still clears what this browser holds.
    setCleanupFlag()
    try {
      await confirmDeletion.mutateAsync({ code: trimmed })
    } catch (err) {
      if (isAxiosError(err) && err.response?.status === 401) {
        if (earlierAttemptPending) {
          finishDeletionCleanup()
          return
        }
        // the session was revoked before this attempt reached the server: no mark was made
        clearCleanupFlag()
        expireSession()
        return
      }
      // Without a response the server may have committed, so the flag stays.
      if (isAxiosError(err) && err.response) {
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
          {shownExport === 'done' ? <p className="text-sm text-muted-foreground">{t('user.delete_data.confirm.export.done')}</p> : null}
          {shownExport === 'none' ? <p className="text-sm text-muted-foreground">{t('user.delete_data.confirm.export.none')}</p> : null}
          {shownExport === 'failed' ? (
            <div className="flex flex-col items-start gap-2">
              <p className="text-sm text-destructive">{t('user.delete_data.confirm.export.failed')}</p>
              <Button type="button" variant="secondary" disabled={!exportReady} onClick={() => void retryDownload()}>
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
            // a failed download never blocks: the user was told and may go ahead
            disabled={confirming || !online || !queueSettled || waiting > 0 || shownExport === 'idle' || shownExport === 'running'}
          >
            {t('user.delete_data.confirm.action')}
          </Button>
        </form>
      )}

      <LoadingDialog open={requestDeletion.isPending} label={t('user.change_email.loading.label')} />
    </SettingsShell>
  )
}
