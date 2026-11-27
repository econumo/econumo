import { useCallback, useEffect, useRef, useState } from 'react'
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
import { DELETION_CLEANUP_KEY, deletionConfirmPending, expireSession, finishDeletionCleanup, setDeletionConfirmInFlight } from '@/api/client'
import { getAccountListWithin } from '@/api/account'
import { exportTransactionList } from '@/api/transaction'
import type { Id } from '@/api/types'
import { RouterPage } from '@/app/router-pages'
import { useOpenBillingPortal } from '@/features/access/useOpenBillingPortal'
import { signOutLocally } from '@/features/auth/localSignOut'
import { usePasswordLoginAvailable } from '@/features/auth/oauthQueries'
import { useStoredQueueSettled, useWaitingCount } from '@/features/pendingWrites/hooks'
import { useConfirmUserDeletion, useRequestUserDeletion, useUserData } from '@/features/user/queries'
import { SettingsShell } from './SettingsShell'

type ExportState = 'idle' | 'running' | 'done' | 'failed' | 'none'

// generous: a large export is slow, but a stalled one must end as failed
// rather than keep Confirm disabled
const EXPORT_TIMEOUT_MS = 5 * 60_000
// The export's account list is its own request, outside the app's retry policy
// (which retries an unreachable backend forever): any failure must end as a
// failed export with Retry, never as a download that runs for good.
const ACCOUNT_LIST_TIMEOUT_MS = 30_000
// A hung confirm would keep the in-flight marker set, muting every other 401.
const CONFIRM_TIMEOUT_MS = 60_000

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
  // the CSV must hold every change, so it waits for the queue to be known and empty
  const exportReady = phase === 'confirm' && queueSettled && waiting === 0 && !!userId

  const download = useCallback(async (ids: Id[]) => {
    if (ids.length === 0) {
      setExportState('none')
      return
    }
    setExportState('running')
    try {
      const blob = await exportTransactionList(ids, { timeout: EXPORT_TIMEOUT_MS })
      downloadBlob(blob, `transactions-${localDateStamp()}.csv`)
      setExportState('done')
    } catch {
      setExportState('failed')
    }
  }, [])

  // One path for the first run and for Retry. The account list is fetched
  // fresh: a cached one can miss an account made on another device, so a
  // failed fetch is a failed export, never a silent fallback to the cache. It
  // is read straight from the API, so the shared accounts query is untouched.
  const runExport = useCallback(async () => {
    exportStarted.current = true
    setExportState('running')
    let fresh: Awaited<ReturnType<typeof getAccountListWithin>>
    try {
      fresh = await getAccountListWithin(ACCOUNT_LIST_TIMEOUT_MS)
    } catch {
      setExportState('failed')
      return
    }
    await download(ownedAccountIds(fresh, userId))
  }, [download, userId])

  useEffect(() => {
    if (!exportReady || exportStarted.current) {
      return
    }
    void runExport()
  }, [exportReady, runExport])

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
    setDeletionConfirmInFlight(true)
    let confirmed = false
    try {
      await confirmDeletion.mutateAsync({ code: trimmed, timeout: CONFIRM_TIMEOUT_MS })
      confirmed = true
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
      // Only a definite refusal clears the flag: without a response, or with a
      // 5xx (a proxy's 502/504 can arrive after the commit), the mark may exist.
      const status = isAxiosError(err) ? err.response?.status : undefined
      if (status !== undefined && status >= 400 && status < 500) {
        clearCleanupFlag()
      }
      setConfirmError(apiErrorMessage(err))
      setConfirming(false)
      return
    } finally {
      // on success the marker stays until this page's own clean-up is done,
      // so a sibling 401 cannot start a second one meanwhile
      if (!confirmed) {
        setDeletionConfirmInFlight(false)
      }
    }
    // the server has revoked every session: a closed tab must not leave the data behind
    try {
      await signOutLocally()
    } finally {
      setDeletionConfirmInFlight(false)
    }
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
              <Button type="button" variant="secondary" disabled={!exportReady} onClick={() => void runExport()}>
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
            disabled={confirming || !online || !queueSettled || waiting > 0 || exportState === 'running'}
          >
            {t('user.delete_data.confirm.action')}
          </Button>
        </form>
      )}

      <LoadingDialog open={requestDeletion.isPending} label={t('user.change_email.loading.label')} />
    </SettingsShell>
  )
}
